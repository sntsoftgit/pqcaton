// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build guidetest

package main

// 안내서 명령 시험(IC-U108)이 만드는 바이너리에만 들어간다(-tags guidetest). PQCOTA_DSN이 memory: 로 시작하면 빈 메모리
// pqcota 이력으로 뜬다. 제품 바이너리는 이 태그 없이 빌드하므로 이 길이 없다.

import (
	"strings"

	"github.com/randyinthedev-hash/pqcota-inventory/pkg/inventory/history"

	"github.com/sntsoftgit/pqcaton/pkg/inventory/upstream"
)

type memHistory struct{ upstream.Lookup }

func (memHistory) Close() {}

func init() {
	real := openUpstream
	openUpstream = func(dsn, org string) (upstream.Store, string, error) {
		if strings.HasPrefix(dsn, "memory:") {
			return memHistory{history.NewMemStore()}, org, nil
		}
		return real(dsn, org)
	}
}
