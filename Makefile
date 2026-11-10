# Barney — kısayollar. Arayüz (web/) derlenip ikiliye gömülür; `make build` sonrası tek dosya: bin/barney.
GO   ?= GOTOOLCHAIN=go1.27.3 go
PORT ?= 8777
VITE ?= 5173

.PHONY: dev run build web go-build stop test doctor ext url clean

## make dev — Go sunucusu + Vite (canlı yenileme). Açılacak adres terminale yazılır.
dev: go-build stop
	@echo; echo "Arayüz (geliştirme):"; ./bin/barney url --dev --port $(PORT) --vite $(VITE); echo
	@trap 'kill 0' INT TERM EXIT; \
	  ./bin/barney serve --port $(PORT) >/dev/null & \
	  (cd web && pnpm exec vite --host 127.0.0.1 --port $(VITE) --strictPort) & \
	  wait

## make run — derle (arayüz dahil) ve tek ikiliyle çalıştır.
run: build stop
	./bin/barney serve --port $(PORT)

build: web go-build

web:
	cd web && pnpm install --frozen-lockfile && pnpm build

go-build:
	$(GO) build -o bin/barney ./cmd/barney

stop:
	-@fuser -k $(PORT)/tcp >/dev/null 2>&1; sleep 0.5

ext: go-build
	./bin/barney extension install

url: go-build
	./bin/barney url --port $(PORT)

doctor: go-build
	./bin/barney doctor --port $(PORT)

test:
	$(GO) vet ./... && $(GO) test -race ./... && node --test extension/test/pure.test.cjs && cd web && pnpm test --run

clean:
	rm -rf bin web/dist/*; touch web/dist/.gitkeep
