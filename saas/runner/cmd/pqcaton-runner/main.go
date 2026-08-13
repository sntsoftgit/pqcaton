// Command pqcaton-runner — 스케줄이 부르는 러너.
//
// 조립만 한다. 판단은 saas/runner 패키지에 있다.
//
//	pqcaton-runner -conf /etc/pqcaton/runner.conf
//
// 상주하지 않는다. 한 번 돌고 끝난다 — 다음은 스케줄이 부른다.
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/sntsoftgit/pqcaton/saas/runner"
)

func main() {
	conf := flag.String("conf", "/etc/pqcaton/runner.conf", "설정 파일")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := runner.LoadConfig(*conf)
	if err != nil {
		// 무엇이 없는지는 말하되 토큰은 담지 않는다 — 이 줄이 로그에 남는다.
		log.Error("설정을 읽을 수 없다", "conf", *conf, "err", err)
		os.Exit(2)
	}

	rep, err := runner.RunOnce(cfg, runner.NewClient(cfg), log)
	if err != nil {
		// **실패를 숨기지 않는다.** 조용히 0으로 끝내면 스케줄러가 잘 돈 것으로 읽는다.
		log.Error("돌지 못했다", "err", err, "job", rep.JobID)
		os.Exit(1)
	}
	log.Info("끝", "job", rep.JobID, "files", rep.Files, "accepted", rep.Accepted, "job_result", rep.Job)
}
