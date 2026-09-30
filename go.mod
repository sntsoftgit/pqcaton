module github.com/sntsoftgit/pqcaton

go 1.26.4

// pqcota — 관측·정규화·전환물 생성. 이 리포는 그 가운데 세 모듈을 소비한다: 계약과 공통
// 코드(common), 인벤토리 단계(inventory), 로컬 스캔이 부르는 수집기(discovery).
//
// **셋은 같은 태그로 함께 올린다.** 단계 모듈은 따로 릴리스하지 않아서, 하나만 올리면
// 상류가 검증한 적 없는 조합이 된다. `replace` 없이 공개 태그로 받는다.
require (
	github.com/randyinthedev-hash/pqcota-common v0.10.0
	github.com/randyinthedev-hash/pqcota-discovery v0.10.0
	github.com/randyinthedev-hash/pqcota-inventory v0.10.0
)

require (
	github.com/jackc/pgx/v5 v5.10.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/a-h/templ v0.3.1020
	github.com/go-chi/chi/v5 v5.3.1
	golang.org/x/net v0.56.0
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	google.golang.org/grpc v1.82.1 // indirect
)
