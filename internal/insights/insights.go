// Package insights bir sohbetten LLM'siz istatistikler çıkarır.
package insights

import (
	"fmt"
	"sort"
	"time"

	"github.com/MustafaKemal0146/barney/internal/model"
)

// Week bir haftanın mesaj sayıları.
type Week struct {
	Week    string `json:"week"`
	Owner   int    `json:"owner"`
	Partner int    `json:"partner"`
}

// Result analiz sonucudur.
type Result struct {
	Total                 int     `json:"total"`
	OwnerSharePct         float64 `json:"owner_share_pct"`
	PartnerSharePct       float64 `json:"partner_share_pct"`
	InitiatorOwnerPct     float64 `json:"initiator_owner_pct"`
	LatencyOwnerMedianSec float64 `json:"latency_owner_median_sec"`
	LatencyPartnerMedSec  float64 `json:"latency_partner_median_sec"`
	ByHour                [24]int `json:"by_hour"`
	ByWeekday             [7]int  `json:"by_weekday"` // 0 = pazartesi
	Weekly                []Week  `json:"weekly"`
	LastTS                int64   `json:"last_ts"`
	SilenceDays           int     `json:"silence_days"`
}

const (
	newConvGap  = 6 * time.Hour
	maxReplyGap = 12 * time.Hour
)

func median(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	sort.Float64s(x)
	return x[len(x)/2]
}

// Analyze mesajları (herhangi sırada) çözümler. now silenceDays için kullanılır.
func Analyze(msgs []model.Message, owner string, now time.Time) Result {
	var r Result
	ms := append([]model.Message(nil), msgs...)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].TS.Before(ms[j].TS) })
	ownerN := model.Norm(owner)
	isOwner := func(m model.Message) bool { return model.Norm(m.Sender) == ownerN }
	var ownerCnt, partnerCnt, starts, ownerStarts int
	var ownerLat, partnerLat []float64
	weeks := map[string]*Week{}
	var prev *model.Message
	for i := range ms {
		m := ms[i]
		if m.Kind != model.KindText && m.Kind != model.KindMedia {
			continue
		}
		r.Total++
		own := isOwner(m)
		if own {
			ownerCnt++
		} else {
			partnerCnt++
		}
		r.ByHour[m.TS.Hour()]++
		r.ByWeekday[(int(m.TS.Weekday())+6)%7]++
		y, w := m.TS.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", y, w)
		wk := weeks[key]
		if wk == nil {
			wk = &Week{Week: key}
			weeks[key] = wk
		}
		if own {
			wk.Owner++
		} else {
			wk.Partner++
		}
		if prev == nil || m.TS.Sub(prev.TS) >= newConvGap {
			starts++
			if own {
				ownerStarts++
			}
		} else if isOwner(*prev) != own && m.TS.Sub(prev.TS) <= maxReplyGap {
			sec := m.TS.Sub(prev.TS).Seconds()
			if own {
				ownerLat = append(ownerLat, sec)
			} else {
				partnerLat = append(partnerLat, sec)
			}
		}
		p := ms[i]
		prev = &p
		if ts := m.TS.Unix(); ts > r.LastTS {
			r.LastTS = ts
		}
	}
	pc := func(a, b int) float64 {
		if b == 0 {
			return 0
		}
		return float64(int(float64(a)/float64(b)*1000+0.5)) / 10
	}
	r.OwnerSharePct, r.PartnerSharePct = pc(ownerCnt, r.Total), pc(partnerCnt, r.Total)
	r.InitiatorOwnerPct = pc(ownerStarts, starts)
	r.LatencyOwnerMedianSec, r.LatencyPartnerMedSec = median(ownerLat), median(partnerLat)
	keys := make([]string, 0, len(weeks))
	for k := range weeks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 12 {
		keys = keys[len(keys)-12:]
	}
	r.Weekly = []Week{}
	for _, k := range keys {
		r.Weekly = append(r.Weekly, *weeks[k])
	}
	if r.LastTS > 0 {
		r.SilenceDays = max(0, int(now.Sub(time.Unix(r.LastTS, 0)).Hours()/24))
	}
	return r
}
