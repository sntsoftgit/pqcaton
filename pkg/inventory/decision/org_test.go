package decision_test

import (
	"errors"
	"testing"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/decision"
	"github.com/sntsoftgit/pqcaton/pkg/org"
)

// 격리가 있는지가 아니라 **격리를 끌 수 없는지**를 본다.
// 조직 없이 저장소를 여는 경로가 있으면, 언젠가 그 경로로 데이터가 섞인다.
func TestStoreCannotOpenWithoutOrg(t *testing.T) {
	if _, err := decision.NewMemJudgmentStore(""); !errors.Is(err, org.ErrEmpty) {
		t.Fatalf("조직 없이 저장소가 열렸다: %v", err)
	}
}

// 같은 subject라도 조직이 다르면 서로 보이지 않아야 한다.
// 인벤토리는 한 조직의 암호 자산 지도다 — 섞이는 것 자체가 사고다.
func TestOrgsDoNotSeeEachOther(t *testing.T) {
	acme, err := decision.NewMemJudgmentStore("acme")
	if err != nil {
		t.Fatal(err)
	}
	globex, err := decision.NewMemJudgmentStore("globex")
	if err != nil {
		t.Fatal(err)
	}

	// 두 조직이 우연히 같은 subject를 쓴다 — 노드 이름은 조직마다 겹칠 수 있다.
	if err := acme.Save(&decision.Judgment{ID: "a1", Subject: "web-gw", Conclusion: "허용"}); err != nil {
		t.Fatal(err)
	}
	if err := globex.Save(&decision.Judgment{ID: "g1", Subject: "web-gw", Conclusion: "제거"}); err != nil {
		t.Fatal(err)
	}

	got, err := acme.BySubject("web-gw")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a1" {
		t.Fatalf("acme가 자기 판정만 봐야 한다: %+v", got)
	}

	all, err := globex.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].ID != "g1" {
		t.Fatalf("globex가 자기 판정만 봐야 한다: %+v", all)
	}
}
