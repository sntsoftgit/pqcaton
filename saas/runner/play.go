package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// ErrNoPlaybook — 플레이북이 설정되지 않았다.
var ErrNoPlaybook = errors.New("플레이북이 설정되지 않았다")

// DefaultAnsible — 부를 명령. PATH에서 찾는다.
const DefaultAnsible = "ansible-playbook"

// playDeadlineMargin — 점유 만료보다 이만큼 먼저 끊는다.
//
// 만료 시각에 딱 맞춰 끊으면 결과를 올릴 시간이 없다. 그 사이에 회수되면 관측을 해 놓고도
// 못 올린다.
const playDeadlineMargin = 30 * time.Second

// runPlaybook — pqcota의 참조 플레이북을 돌린다.
//
// **우리가 SSH를 하지 않는다.** 반입·실행·회수·정리는 그 플레이북이 하고, 러너는 부르기만
// 한다 — pqcota가 *"자체 원격 실행 엔진을 두지 않는다"*고 정한 것을 그대로 따른다.
//
// 대상은 **`--limit`으로 좁힌다.** 작업이 노드를 지목했는데 인벤토리 전체를 돌리면, 「이
// 노드만」이라는 지시가 뜻을 잃고 과금 대상도 늘어난다.
//
// **점유 만료를 데드라인으로 쓴다.** 만료된 뒤에 끝나 봐야 그 작업은 이미 회수됐다 —
// 그때까지 도는 것은 대상 노드에 부담만 준다.
func runPlaybook(c Config, job Job, log *slog.Logger) error {
	if c.Playbook == "" {
		return ErrNoPlaybook
	}
	bin := c.Ansible
	if bin == "" {
		bin = DefaultAnsible
	}

	args := []string{}
	if c.Inventory != "" {
		args = append(args, "-i", c.Inventory)
	}
	if len(job.Targets) > 0 {
		args = append(args, "--limit", strings.Join(job.Targets, ","))
	}
	args = append(args, c.Playbook)

	ctx := context.Background()
	if !job.LeaseTill.IsZero() {
		deadline := job.LeaseTill.Add(-playDeadlineMargin)
		if !deadline.After(time.Now()) {
			// 받자마자 이미 만료 직전이다. 시작하지 않는 편이 낫다 — 대상 노드에
			// 부담만 주고 결과는 못 올린다.
			return fmt.Errorf("점유가 곧 만료된다(%s) — 시작하지 않는다", job.LeaseTill.Format(time.RFC3339))
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}

	log.Info("플레이북을 돌린다", "job", job.ID, "playbook", c.Playbook,
		"targets", len(job.Targets), "lease_till", job.LeaseTill)

	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// **출력을 통째로 삼키지 않는다.** 왜 실패했는지는 거기에만 있다. 다만 길 수
		// 있으므로 꼬리만 남긴다 — 원인은 대개 끝에 있다.
		log.Error("플레이북이 실패했다", "job", job.ID, "err", err, "tail", tail(out, 2000))
		return fmt.Errorf("%s: %w", bin, err)
	}
	log.Info("플레이북이 끝났다", "job", job.ID)
	return nil
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
