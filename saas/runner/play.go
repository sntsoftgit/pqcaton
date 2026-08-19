package runner

import (
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// ErrNoPlaybook — 플레이북이 설정되지 않았다.
var ErrNoPlaybook = errors.New("no playbook is configured")

// DefaultAnsible — 부를 명령. PATH에서 찾는다.
const DefaultAnsible = "ansible-playbook"

// runPlaybook — pqcota의 참조 플레이북을 돌린다.
//
// **우리가 SSH를 하지 않는다.** 반입·실행·회수·정리는 그 플레이북이 하고, 러너는 부르기만
// 한다 — pqcota가 *"자체 원격 실행 엔진을 두지 않는다"*고 정한 것을 그대로 따른다.
//
// **대상은 인벤토리가 정한다.** 운영자가 채운 그 파일이 이 영역의 대상 전부이고, 관측
// 주기는 스케줄이 정한다 — 컨트롤 플레인이 노드를 지목하지 않는다.
func runPlaybook(c Config, log *slog.Logger) error {
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
	args = append(args, c.Playbook)

	log.Info("running the playbook", "playbook", c.Playbook, "inventory", c.Inventory)

	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		// **출력을 통째로 삼키지 않는다.** 왜 실패했는지는 거기에만 있다. 다만 길 수
		// 있으므로 꼬리만 남긴다 — 원인은 대개 끝에 있다.
		log.Error("the playbook failed", "err", err, "tail", tail(out, 2000))
		return fmt.Errorf("%s: %w", bin, err)
	}
	log.Info("the playbook finished")
	return nil
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
