package ui

import "fmt"

// 저장·확정 뒤에 화면에 뜨는 알림. **명령이 만들되 말은 화면의 것을 쓴다** —
// 이 문장들은 화면 위 초록 상자에 뜨지, 표준오류로 나가지 않는다.
func MsgSavedNotFinal(l Lang) string { return tSavedNotFinal.In(l) }

func MsgFinalizedPlan(l Lang, actions int, path string) string {
	return fmt.Sprintf(tFinalizedPlan.In(l), actions, path)
}

func MsgFinalizedPolicy(l Lang, rules int, path string) string {
	return fmt.Sprintf(tFinalizedPolicy.In(l), rules, path)
}

func MsgJudgmentsSaved(l Lang, n int, path string) string {
	return fmt.Sprintf(tJudgmentsSaved.In(l), n, path)
}

func MsgRulesSaved(l Lang, layers, rules, changes, audited int) string {
	return fmt.Sprintf(tRulesSaved.In(l), rules, layers, changes, audited)
}

func MsgSignatureCleared(l Lang) string { return tSignatureCleared.In(l) }

func MsgDeclSaved(l Lang, nodes, assets, edges int) string {
	return fmt.Sprintf(tDeclSaved.In(l), nodes, assets, edges)
}

func MsgDeclStillOff(l Lang, n int) string { return fmt.Sprintf(tDeclStillOff.In(l), n) }
