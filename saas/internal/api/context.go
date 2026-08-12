package api

import (
	"context"
	"net/http"

	"github.com/pqcota/pqcota/pkg/org"
)

// ctxWith·orgOf — 인증이 유도한 조직을 핸들러까지 나르는 통로.
//
// 핸들러가 요청 본문이나 질의 문자열에서 조직을 읽을 방법을 두지 않는다. 읽을 수
// 있으면 언젠가 읽게 되고, 그 순간 조직이 **러너가 주장하는 것**이 된다(§6.4).
func ctxWith(ctx context.Context, k ctxKey, v any) context.Context {
	return context.WithValue(ctx, k, v)
}

func orgOf(r *http.Request) org.ID {
	o, _ := r.Context().Value(ctxOrg).(org.ID)
	return o
}
