.PHONY: all check-licenses check-text check-prose check-cases check-fmt build test generate verify-generated

all: check-licenses check-text check-prose check-cases check-fmt build test

# 라이선스 관문: 듀얼 라이선스를 실제로 지키는 장치.
# 카피레프트가 하나라도 링크되면 상업 라이선스로 낼 수 없다(→ CONTRIBUTING.md).
check-licenses:
	@go run ./tools/checklicenses

# 코드와 그 출력은 영어, 화면만 두 말이다(CONTRIBUTING.md). 문자열 하나를 옮기지
# 않으면 그 자리만 한국어로 뜨는데 눈으로는 못 찾는다. 그래서 파서로 본다.
check-text:
	@go run ./tools/checktext

# 문체 관문: 문서와 화면 문구의 한국어. **한 번 걷어낸 말이 다시 들어오지 않게** 막는다.
#
# 눈으로 지킨 규칙은 남지 않는다. 폼에 넣는 문서는 check-form-text.py 가 막고 있어 엠대시가
# 하나도 없는데, 그 관문이 보지 않는 문서에는 천 개가 넘게 쌓여 있었다.
#
# **지금 있는 것은 기준선에 적어 두고 늘어나는 것만 막는다**(tools/checkprose/baseline.tsv).
# 고쳐서 줄었으면 `go run ./tools/checkprose -baseline` 으로 기준선을 내리고 함께 커밋한다.
# 그러지 않으면 관문이 「낡았다」고 막는다. 걷어낸 자리가 도로 채워지는 것을 그렇게 잡는다.
check-prose:
	@go run ./tools/checkprose

# 케이스 관문: docs/testcases.md 의 번호와 실제 테스트를 맞댄다.
#
# 문서가 「케이스 번호가 곧 테스트 파일 링크입니다」라고 약속해 놓고 백일흔넷 가운데 링크가
# 하나도 없었다. 약속을 사람이 지키게 두면 이렇게 된다.
#
# 링크는 손으로 붙이지 않는다. `go run ./tools/checkcases -write` 가 찍는다. 손으로 붙이면
# 파일을 옮기는 날 백일흔넷이 한꺼번에 썩는다.
check-cases:
	@go run ./tools/checkcases

# 서식 관문: `gofmt` 가 고칠 것이 남아 있으면 멈춘다.
#
# **문구만 고치는 날에 아무 표시 없이 어긋난다.** var 블록의 정렬은 이름 하나가 길어지면 옆 줄까지
# 함께 움직이는데, 빌드도 테스트도 그것을 보지 않는다. 며칠 쌓이면 그 다음 diff 에서 진짜
# 바뀐 줄이 정렬 줄에 묻힌다.
check-fmt:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
		echo "✗ gofmt 가 고칠 것이 남았다. gofmt -w . 로 맞출 것:"; echo "$$out"; exit 1; \
	fi; \
	echo "✓ gofmt check passed"

build:
	go build ./...

test:
	go test ./... -count=1

# ── 화면 템플릿 ────────────────────────────────────────────────────────────
#
# templ 이 .templ 을 *_templ.go 로 옮긴다. **생성물을 리포에 함께 둔다.** 그래서
# 빌드에는 생성기가 필요 없고, 이 리포를 `go get` 하는 쪽도 도구를 깔지 않는다.
# 화면(.templ)을 고친 사람만 이것을 돌리고 결과를 함께 커밋한다.
TEMPL := github.com/a-h/templ/cmd/templ@v0.3.1020

generate:
	go run $(TEMPL) generate

# 고쳐 놓고 안 돌린 날을 잡는다. 그대로 두면 **화면은 예전 것이 뜨는데 소스는 새것**이라,
# 무엇을 보고 있는지가 어긋난다. all 에 넣지 않은 것은 이것만 망을 타기 때문이다.
verify-generated:
	@go run $(TEMPL) generate
	@git diff --exit-code -- '*_templ.go' \
		|| { echo "✗ .templ 을 고치고 make generate 를 안 돌렸다. 생성물을 함께 커밋할 것"; exit 1; }
