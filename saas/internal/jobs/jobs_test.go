package jobs_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pqcota/pqcota/pkg/org"

	"github.com/sntsoftgit/pqcaton/saas/internal/jobs"
)

const acme = org.ID("acme")

var t0 = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)

func put(t *testing.T, s jobs.Store, id string, k jobs.Kind, created time.Time) {
	t.Helper()
	if err := s.Put(jobs.Job{ID: id, Org: acme, Kind: k, State: jobs.Pending, Created: created}); err != nil {
		t.Fatalf("넣기: %v", err)
	}
}

// CP-JOB-1 — 대기 중 작업을 받아가면 점유가 된다.
func TestLeaseMarksLeased(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "j1", jobs.Observe, t0)

	j, ok, err := s.Lease(acme, "r1", t0.Add(time.Minute), t0)
	if err != nil || !ok {
		t.Fatalf("점유: %v %v", ok, err)
	}
	if j.State != jobs.Leased || j.RunnerID != "r1" || j.Attempts != 1 {
		t.Fatalf("점유 상태가 아니다: %+v", j)
	}
	if _, again, _ := s.Lease(acme, "r2", t0.Add(time.Minute), t0); again {
		t.Fatal("점유된 작업이 다른 러너에게도 나갔다")
	}
}

// CP-JOB-2 — **다른 조직의 작업은 받아갈 수 없다.**
// 러너의 조직은 토큰에서 유도된 것이고, 작업 조회에 그 조건이 붙어야 경계가 선다.
func TestLeaseIsolatesOrg(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "j1", jobs.Observe, t0)

	if _, ok, _ := s.Lease("beta", "r1", t0.Add(time.Minute), t0); ok {
		t.Fatal("다른 조직의 작업이 나갔다")
	}
	if _, err := s.Get("beta", "j1"); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("다른 조직에서 조회됐다: %v", err)
	}
}

// CP-JOB-3 — 오래 기다린 것부터 나간다.
// 순서가 흔들리면 굶는 작업이 생겨도 보이지 않는다.
func TestLeaseTakesOldestFirst(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "new", jobs.Observe, t0.Add(time.Hour))
	put(t, s, "old", jobs.Observe, t0)

	j, _, _ := s.Lease(acme, "r1", t0.Add(time.Minute), t0)
	if j.ID != "old" {
		t.Fatalf("오래된 것이 먼저 나가지 않았다: %s", j.ID)
	}
}

// CP-JOB-4 — 점유하지 않은 러너는 끝냈다고 말할 수 없다.
func TestCompleteRequiresOwnLease(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "j1", jobs.Observe, t0)
	if _, _, err := s.Lease(acme, "r1", t0.Add(time.Minute), t0); err != nil {
		t.Fatalf("점유: %v", err)
	}
	if err := s.Complete(acme, "j1", "r2", t0); !errors.Is(err, jobs.ErrNotLeased) {
		t.Fatalf("남의 작업을 끝냈다: %v", err)
	}
	if err := s.Complete(acme, "j1", "r1", t0); err != nil {
		t.Fatalf("완료: %v", err)
	}
	j, _ := s.Get(acme, "j1")
	if j.State != jobs.Done {
		t.Fatalf("완료가 반영되지 않았다: %+v", j)
	}
}

// CP-JOB-5 — 진행 중임을 알리면 만료가 미뤄진다.
// 없으면 긴 작업에 억지로 큰 만료값을 잡아야 하고, 그러면 죽은 러너의 작업이 오래 묶인다.
func TestExtendPushesLease(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "j1", jobs.Observe, t0)
	_, _, _ = s.Lease(acme, "r1", t0.Add(time.Minute), t0)

	if err := s.Extend(acme, "j1", "r2", t0.Add(time.Hour)); !errors.Is(err, jobs.ErrNotLeased) {
		t.Fatalf("남의 점유가 미뤄졌다: %v", err)
	}
	if err := s.Extend(acme, "j1", "r1", t0.Add(time.Hour)); err != nil {
		t.Fatalf("연장: %v", err)
	}
	// 원래 만료를 지나도 아직 점유 상태여야 한다.
	back, review, _ := s.Sweep(t0.Add(2 * time.Minute))
	if back != 0 || review != 0 {
		t.Fatalf("연장했는데 회수됐다: 재배포 %d, 확인필요 %d", back, review)
	}
}

// CP-JOB-6 — 만료된 **읽기** 작업은 다시 대기로 간다.
func TestSweepRedeploysReadJobs(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "obs", jobs.Observe, t0)
	put(t, s, "enr", jobs.Enroll, t0)
	_, _, _ = s.Lease(acme, "r1", t0.Add(time.Minute), t0)
	_, _, _ = s.Lease(acme, "r1", t0.Add(time.Minute), t0)

	back, review, err := s.Sweep(t0.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("정리: %v", err)
	}
	if back != 2 || review != 0 {
		t.Fatalf("읽기 작업이 돌아오지 않았다: 재배포 %d, 확인필요 %d", back, review)
	}
	for _, id := range []string{"obs", "enr"} {
		j, _ := s.Get(acme, id)
		if j.State != jobs.Pending || j.RunnerID != "" {
			t.Fatalf("%s가 대기로 돌아가지 않았다: %+v", id, j)
		}
	}
}

// CP-JOB-7 — **만료된 `provision`은 자동으로 다시 주지 않는다.**
//
// 러너가 플레이북을 이미 적용했는데 응답만 못 준 것일 수 있다. 다시 주면 두 번 적용되고,
// 두 번째 before 캡처는 **이미 바뀐 상태**를 찍어 되돌릴 근거가 망가진다(§6.2.1).
func TestSweepDoesNotRedeployProvision(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "prov", jobs.Provision, t0)
	_, _, _ = s.Lease(acme, "r1", t0.Add(time.Minute), t0)

	back, review, err := s.Sweep(t0.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("정리: %v", err)
	}
	if back != 0 || review != 1 {
		t.Fatalf("적용 작업이 자동 재배포됐다: 재배포 %d, 확인필요 %d", back, review)
	}
	j, _ := s.Get(acme, "prov")
	if j.State != jobs.NeedsReview {
		t.Fatalf("사람이 볼 자리로 가지 않았다: %+v", j)
	}
	if j.Note == "" {
		t.Fatal("왜 멈췄는지가 남지 않았다 — 조용히 멈추면 잊힌다")
	}
	// 그리고 다시 배포되지 않는다.
	if _, ok, _ := s.Lease(acme, "r2", t0.Add(time.Hour), t0.Add(3*time.Minute)); ok {
		t.Fatal("확인 필요 상태인데 다시 나갔다")
	}
}

// CP-JOB-8 — 만료 전에는 회수하지 않는다.
func TestSweepLeavesLiveLeases(t *testing.T) {
	s := jobs.NewMemStore()
	put(t, s, "j1", jobs.Observe, t0)
	_, _, _ = s.Lease(acme, "r1", t0.Add(time.Hour), t0)

	back, review, _ := s.Sweep(t0.Add(time.Minute))
	if back != 0 || review != 0 {
		t.Fatalf("살아 있는 점유를 뺏었다: %d %d", back, review)
	}
}

// CP-JOB-9 — 조직 없이 열리는 경로가 없다.
func TestJobsRequireOrg(t *testing.T) {
	s := jobs.NewMemStore()
	if err := s.Put(jobs.Job{ID: "j1", Kind: jobs.Observe}); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 넣었다: %v", err)
	}
	if _, _, err := s.Lease("", "r1", t0, t0); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 점유했다: %v", err)
	}
	if _, err := s.Get("", "j1"); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 조회했다: %v", err)
	}
}

// CP-JOB-10 — 종류가 재배포 가능 여부를 안다. 정책이 한 자리에만 있어야 한다.
//
// **모르는 종류는 재배포하지 않는다.** `!= Provision`으로 쓰면 새 종류가 자동 재배포로
// 떨어진다 — 그것이 쓰기 작업이면 아무도 손대지 않아도 두 번 적용되는 쪽으로 기운다.
func TestRedeployablePolicy(t *testing.T) {
	if !jobs.Observe.Redeployable() || !jobs.Enroll.Redeployable() {
		t.Fatal("읽기 작업이 재배포 불가로 잡혔다")
	}
	if jobs.Provision.Redeployable() {
		t.Fatal("쓰기 작업이 재배포 가능으로 잡혔다")
	}
	if jobs.Kind("아직-없는-종류").Redeployable() {
		t.Fatal("모르는 종류가 재배포 가능으로 잡혔다")
	}
	// AllKinds가 뒤처지면 Pg판의 정리 질의가 아는 종류를 빠뜨린다.
	if len(jobs.AllKinds) != 3 {
		t.Fatalf("AllKinds가 종류를 따라가지 못한다: %v", jobs.AllKinds)
	}
}
