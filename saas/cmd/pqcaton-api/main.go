// Command pqcaton-api — 러너 수신 API.
//
// 조립만 한다. 판단은 saas/internal의 각 패키지에 있다.
//
//	env PQCATON_DSN        : Postgres DSN (필수)
//	env PQCATON_ADDR       : 듣는 주소 (기본 127.0.0.1:8080)
//	env PQCATON_TRUST_PROXY: "1"이면 X-Forwarded-Proto를 본다
//	  -migrate             : 스키마를 올리고 끝낸다
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/pqcota/pqcota/pkg/discovery/history"
	"github.com/pqcota/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/saas/internal/access"
	"github.com/sntsoftgit/pqcaton/saas/internal/api"
	"github.com/sntsoftgit/pqcaton/saas/internal/intake"
)

func main() {
	migrate := flag.Bool("migrate", false, "스키마를 올리고 끝낸다")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dsn := os.Getenv("PQCATON_DSN")
	if dsn == "" {
		log.Error("PQCATON_DSN이 없다")
		os.Exit(2)
	}
	ctx := context.Background()

	// 스키마 배포는 의도적 행위다 — 서버가 뜨면서 말없이 DDL을 돌지 않는다.
	if *migrate {
		if err := access.Migrate(ctx, dsn); err != nil {
			log.Error("access 스키마", "err", err)
			os.Exit(1)
		}
		if err := intake.MigrateSeen(ctx, dsn); err != nil {
			log.Error("멱등 스키마", "err", err)
			os.Exit(1)
		}
		log.Info("스키마를 올렸다")
		return
	}

	acc, err := access.NewPgStore(ctx, dsn)
	if err != nil {
		log.Error("access 저장소", "err", err)
		os.Exit(1)
	}
	defer acc.Close()

	seen, err := intake.NewPgSeen(ctx, dsn)
	if err != nil {
		log.Error("멱등 저장소", "err", err)
		os.Exit(1)
	}
	defer seen.Close()

	// 조직마다 히스토리 핸들이 다르다. 여기서 한 번 열고 재사용한다 —
	// 조직 수만큼 풀이 늘어나므로, 고객이 늘면 이 자리를 다시 본다.
	pools := map[org.ID]history.Store{}
	stores := func(o org.ID) (history.Store, error) {
		if s, ok := pools[o]; ok {
			return s, nil
		}
		s, err := history.NewPgStoreIn(ctx, dsn, string(o))
		if err != nil {
			return nil, err
		}
		pools[o] = s
		return s, nil
	}

	srv := api.New(acc, seen, stores, api.Config{
		TrustProxy: os.Getenv("PQCATON_TRUST_PROXY") == "1",
	}, log)

	addr := os.Getenv("PQCATON_ADDR")
	if addr == "" {
		// 기본은 루프백이다. TLS를 앞단에서 끊으므로 이 포트가 인터넷에 열리면
		// 러너 토큰이 평문으로 오간다(러너 설계 §6.2).
		addr = "127.0.0.1:8080"
	}
	h := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Info("듣는다", "addr", addr, "trust_proxy", os.Getenv("PQCATON_TRUST_PROXY") == "1")
	if err := h.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
