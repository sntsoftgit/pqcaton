module github.com/sntsoftgit/pqcaton

go 1.26.4

// pqcota — 관측·정규화·전환물 생성. 이 리포는 그 계약(contracts/)만 소비한다.
//
// ★ replace가 붙은 이유: pqcota의 go.mod가 선언한 모듈 경로(github.com/pqcota/pqcota)와
//   실제 리포 주소(github.com/randyinthedev-hash/pqcota)가 다르다. 그대로는 `go get`이
//   해소하지 못한다. upstream이 모듈 경로를 정리하면 이 줄을 지운다.
require (
	github.com/jackc/pgx/v5 v5.10.0
	github.com/pqcota/pqcota v0.1.2
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
	google.golang.org/grpc v1.82.1 // indirect
)

replace github.com/pqcota/pqcota => github.com/randyinthedev-hash/pqcota v0.1.2
