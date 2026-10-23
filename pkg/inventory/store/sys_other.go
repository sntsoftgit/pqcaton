// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

//go:build !unix

package store

import (
	"errors"
	"io/fs"
	"os"
)

const noFollow = 0

var errWouldBlock = errors.New("would block")

// tryLock: 유닉스가 아니면 잠그지 않는다. 여기까지 오기 전에 platformCheck가 막는다.
func tryLock(*os.File) error { return ErrUnsupported }

func linkCount(fs.FileInfo) uint64 { return 1 }

func ownerOf(fs.FileInfo) (int, bool) { return 0, false }
