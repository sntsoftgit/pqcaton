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

// 작업 종류. 컨트롤 플레인이 정한 이름을 그대로 쓴다(§6.2).
const (
	// KindEnroll — 대상에 붙어 머신 지문만 확인한다.
	KindEnroll = "enroll"
	// KindObserve — collector를 반입·실행·회수한다.
	KindObserve = "observe"
	// KindProvision — 확정된 계획을 적용한다.
	KindProvision = "provision"
)

var (
	// ErrNoPlaybook — 플레이북이 설정되지 않았다.
	ErrNoPlaybook = errors.New("플레이북이 설정되지 않았다")
	// ErrUnknownKind — 모르는 작업 종류다.
	//
	// **아무것도 하지 않는다.** 모르는 것을 관측 플레이북으로 돌리면, 그것이 쓰기
	// 작업일 때 대상 노드에 무슨 일이 날지 우리가 모른다 — 새 종류가 생길 때
	// 옛 러너가 조용히 엉뚱한 일을 하는 것을 막는다(§6.6).
	ErrUnknownKind = errors.New("모르는 작업 종류다")
)

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
	switch job.Kind {
	case KindEnroll, KindObserve:
		// 둘 다 읽기다. pqcota의 참조 플레이북이 반입·실행·회수를 하고, 등재는 그중
		// 지문 확인만 쓴다 — 대상을 좁히는 것 말고 러너가 달리 할 일이 없다.
	case KindProvision:
		// **쓰기다. 아직 하지 않는다.** 확정된 계획을 받아 적용하는 경로는 만들지
		// 않았고, 반쯤 만든 것으로 고객 서버에 쓰지 않는다.
		return fmt.Errorf("%w: %s — 적용은 아직 만들지 않았다", ErrUnknownKind, job.Kind)
	default:
		return fmt.Errorf("%w: %q", ErrUnknownKind, job.Kind)
	}
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
