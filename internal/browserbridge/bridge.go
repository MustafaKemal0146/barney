// Package browserbridge, FETİH sunucusu ile kullanıcının gerçek Chrome'una kurulu FETİH uzantısı
// arasındaki WebSocket köprüsüdür. Ajan `chrome` aracını çağırınca çağrı buradan uzantıya gider,
// uzantı chrome.debugger (CDP) ile eylemi yapar ve sonucu geri yollar.
//
// Köprü bilerek ince tutuldu: tek bir uzantı bağlantısı, istek/yanıt eşleştirme, zaman aşımı,
// sekme başına sıralama ve sekme adreslerinin (kapsam denetimi için) son bilinen hâli.
package browserbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// ExtensionID FETİH Chrome uzantısının sabit kimliğidir (extension/manifest.json "key" alanından türer).
const ExtensionID = "jadjjaknbcpiadkkgkcgapdfehfekgap"

// ProtocolVersion uzantı ile sunucunun uyumlu olduğu çerçeve protokolü sürümüdür.
const ProtocolVersion = "1"

// Hata kodları (Reply.Error.Code). Uzantı da aynı kodları üretir.
const (
	ErrNotConnected = "not_connected"
	ErrTimeout      = "timeout"
	ErrDisconnected = "disconnected"
)

// Call sunucudan uzantıya giden bir araç çağrısıdır.
type Call struct {
	ID   string `json:"id"`
	Type string `json:"type"` // her zaman "call"
	// Action chrome aracının action değeridir (navigate, click, ...).
	Action string `json:"tool"`
	TabID  int    `json:"tabId,omitempty"`
	// ExpectHost doluysa uzantı, eylemden hemen önce sekmenin gerçek adresinin bu host olduğunu
	// doğrular; değilse hiçbir şey yapmadan host_changed döner (onay ile eylem arasındaki yarış).
	ExpectHost string          `json:"expectHost,omitempty"`
	SessionID  string          `json:"sessionId,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
}

// Content bir yanıt parçasıdır: metin ya da görsel.
type Content struct {
	Type string `json:"type"` // "text" | "image"
	Text string `json:"text,omitempty"`
	Mime string `json:"mime,omitempty"`
	Data string `json:"data,omitempty"` // base64 (image)
}

// TabInfo bir sekmenin anlık durumudur.
type TabInfo struct {
	ID    int    `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// Error uzantının döndürdüğü yapılandırılmış hatadır.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// Reply uzantıdan gelen yanıttır.
type Reply struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"` // "result"
	OK      bool      `json:"ok"`
	Content []Content `json:"content,omitempty"`
	Error   *Error    `json:"error,omitempty"`
	Tab     *TabInfo  `json:"tab,omitempty"`
}

// frame, soketten gelen her şeyi ayrıştırmak için gevşek bir zarftır.
type frame struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"` // event adı
	// hello
	Version    string `json:"version"`
	ExtVersion string `json:"extVersion"`
	Browser    string `json:"browser"`
	// event
	Tab    *TabInfo `json:"tab"`
	TabID  int      `json:"tabId"`
	Reason string   `json:"reason"`
}

// Status köprünün anlık durumudur (Ayarlar kartı için).
type Status struct {
	Connected  bool      `json:"connected"`
	ExtVersion string    `json:"ext_version,omitempty"`
	Browser    string    `json:"browser,omitempty"`
	Since      time.Time `json:"since,omitempty"`
	ActiveTab  *TabInfo  `json:"active_tab,omitempty"`
}

// Bridge tek bir uzantı bağlantısını yönetir.
type Bridge struct {
	mu      sync.Mutex
	conn    *websocket.Conn
	connGen int64
	hello   frame
	since   time.Time
	pending map[string]chan Reply
	seq     int64
	tabs    map[int]TabInfo // son bilinen sekme durumları (tab_update olaylarından)
	active  int             // son etkin FETİH sekmesi
	onEvent func(name string, tabID int, reason string)

	tabLocks map[int]chan struct{}
}

// New boş bir köprü oluşturur.
func New() *Bridge {
	return &Bridge{pending: map[string]chan Reply{}, tabs: map[int]TabInfo{}, tabLocks: map[int]chan struct{}{}}
}

// OnEvent uzantıdan gelen olaylar (ör. detached) için bir dinleyici kaydeder.
func (b *Bridge) OnEvent(fn func(name string, tabID int, reason string)) {
	b.mu.Lock()
	b.onEvent = fn
	b.mu.Unlock()
}

// Connected uzantının şu an bağlı olup olmadığını döndürür.
func (b *Bridge) Connected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conn != nil
}

// Status anlık durumu döndürür.
func (b *Bridge) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := Status{Connected: b.conn != nil}
	if b.conn != nil {
		st.ExtVersion, st.Browser, st.Since = b.hello.ExtVersion, b.hello.Browser, b.since
		if t, ok := b.tabs[b.active]; ok {
			t := t
			st.ActiveTab = &t
		}
	}
	return st
}

// TabURL bir sekmenin son bilinen adresini döndürür; tabID 0 ise etkin sekmeyi kullanır.
func (b *Bridge) TabURL(tabID int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if tabID == 0 {
		tabID = b.active
	}
	return b.tabs[tabID].URL
}

// Serve, kabul edilmiş bir WebSocket bağlantısını sahiplenir ve bağlantı kapanana kadar okur.
// Önceki bir bağlantı varsa kapatılır (tek uzantı). Bloklar; çağıran handler bu süre boyunca açık kalır.
func (b *Bridge) Serve(ctx context.Context, c *websocket.Conn) {
	c.SetReadLimit(64 << 20) // ekran görüntüleri/büyük ağaçlar için
	b.mu.Lock()
	old := b.conn
	b.conn = c
	b.connGen++
	gen := b.connGen
	b.since = time.Now()
	b.hello = frame{}
	b.mu.Unlock()
	if old != nil {
		_ = old.Close(websocket.StatusPolicyViolation, "yeni bağlantı")
	}
	defer func() {
		b.mu.Lock()
		if b.connGen == gen {
			b.conn = nil
			b.failAllLocked(ErrDisconnected, "Chrome uzantısı bağlantısı koptu")
		}
		b.mu.Unlock()
		_ = c.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var f frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		switch f.Type {
		case "hello":
			b.mu.Lock()
			if b.connGen == gen {
				b.hello = f
			}
			b.mu.Unlock()
		case "ping":
			_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"pong"}`))
		case "result":
			var r Reply
			if json.Unmarshal(data, &r) != nil {
				continue
			}
			b.mu.Lock()
			ch := b.pending[r.ID]
			delete(b.pending, r.ID)
			if r.Tab != nil {
				b.tabs[r.Tab.ID] = *r.Tab
				b.active = r.Tab.ID
			}
			b.mu.Unlock()
			if ch != nil {
				ch <- r
			}
		case "event":
			b.mu.Lock()
			if f.Tab != nil {
				b.tabs[f.Tab.ID] = *f.Tab
				if f.Name == "tab_active" {
					b.active = f.Tab.ID
				}
			}
			if f.Name == "tab_removed" {
				delete(b.tabs, f.TabID)
			}
			cb := b.onEvent
			b.mu.Unlock()
			if cb != nil && f.Name != "tab_update" && f.Name != "tab_active" {
				cb(f.Name, f.TabID, f.Reason)
			}
		}
	}
}

func (b *Bridge) failAllLocked(code, msg string) {
	for id, ch := range b.pending {
		ch <- Reply{ID: id, Type: "result", Error: &Error{Code: code, Message: msg}}
		delete(b.pending, id)
	}
}

// lockTab aynı sekmeye eşzamanlı çağrıları sıraya koyar (CDP durumu paylaşılıyor).
func (b *Bridge) lockTab(ctx context.Context, tabID int) (func(), error) {
	b.mu.Lock()
	ch := b.tabLocks[tabID]
	if ch == nil {
		ch = make(chan struct{}, 1)
		b.tabLocks[tabID] = ch
	}
	b.mu.Unlock()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// DefaultTimeout bir çağrının varsayılan azami süresidir; navigate daha uzun bekler.
func DefaultTimeout(action string) time.Duration {
	switch action {
	case "navigate", "wait":
		return 60 * time.Second
	}
	return 30 * time.Second
}

// Call bir çağrıyı uzantıya yollar ve yanıtı bekler. Uzantı bağlı değilse, zaman aşımında ya da
// bağlantı koparsa *Error döner.
func (b *Bridge) Call(ctx context.Context, c Call) (Reply, error) {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn == nil {
		return Reply{}, &Error{Code: ErrNotConnected, Message: "Chrome uzantısı bağlı değil. Ayarlar → Chrome uzantısı bölümünden kurup Chrome'u aç (uzantı kendiliğinden bağlanır)."}
	}
	unlock, err := b.lockTab(ctx, c.TabID)
	if err != nil {
		return Reply{}, err
	}
	defer unlock()

	b.mu.Lock()
	b.seq++
	c.ID = "c_" + strconv.FormatInt(b.seq, 10)
	c.Type = "call"
	ch := make(chan Reply, 1)
	b.pending[c.ID] = ch
	b.mu.Unlock()

	data, _ := json.Marshal(c)
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = conn.Write(wctx, websocket.MessageText, data)
	cancel()
	if err != nil {
		b.drop(c.ID)
		return Reply{}, &Error{Code: ErrDisconnected, Message: "Chrome uzantısına yazılamadı: " + err.Error()}
	}

	timer := time.NewTimer(DefaultTimeout(c.Action))
	defer timer.Stop()
	select {
	case r := <-ch:
		if r.Error != nil {
			return r, r.Error
		}
		if !r.OK {
			return r, &Error{Code: "failed", Message: "uzantı eylemi başarısız"}
		}
		return r, nil
	case <-timer.C:
		b.drop(c.ID)
		return Reply{}, &Error{Code: ErrTimeout, Message: fmt.Sprintf("Chrome eylemi zaman aşımına uğradı (%s)", c.Action)}
	case <-ctx.Done():
		b.drop(c.ID)
		b.cancelRemote(conn, c.ID)
		return Reply{}, ctx.Err()
	}
}

func (b *Bridge) drop(id string) {
	b.mu.Lock()
	delete(b.pending, id)
	b.mu.Unlock()
}

// cancelRemote iptal edilen bir çağrıyı uzantıya bildirir (en iyi çaba).
func (b *Bridge) cancelRemote(conn *websocket.Conn, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, _ := json.Marshal(map[string]string{"type": "cancel", "id": id})
	_ = conn.Write(ctx, websocket.MessageText, data)
}

// DetachAll uzantıdan tüm sekmelerdeki debugger'ı ayırmasını ister (Durdur düğmesi / oturum iptali).
func (b *Bridge) DetachAll() {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"detach_all"}`))
}

// IsNotConnected hatanın "uzantı bağlı değil" olup olmadığını söyler.
func IsNotConnected(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == ErrNotConnected
}

// Disconnect mevcut uzantı bağlantısını (varsa) kapatır; bekleyen çağrılar hata ile biter.
func (b *Bridge) Disconnect() {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "Chrome entegrasyonu kapatıldı")
	}
}
