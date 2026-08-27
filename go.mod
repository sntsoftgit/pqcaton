module github.com/sntsoftgit/pqcaton

go 1.26.4

// pqcota — 관측·정규화·전환물 생성. 이 리포는 그 계약(contracts/)만 소비한다.
//
// v0.5.0에서 모듈 경로가 리포 주소에 맞춰졌다. `replace`가 필요 없어졌고, 그래서
// **이 리포를 가져다 쓰는 쪽도 우회를 들고 있지 않아도 된다.**
require (
	github.com/jackc/pgx/v5 v5.10.0
	github.com/randyinthedev-hash/pqcota v0.6.3
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
