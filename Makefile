.PHONY: all check-licenses build test

all: check-licenses build test

# 라이선스 게이트 — 듀얼 라이선스를 실제로 지키는 장치.
# 카피레프트가 하나라도 링크되면 상업 라이선스로 낼 수 없다(→ CONTRIBUTING.md).
check-licenses:
	@go run ./tools/checklicenses

build:
	go build ./...

test:
	go test ./... -count=1
