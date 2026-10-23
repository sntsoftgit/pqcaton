// SPDX-FileCopyrightText: 2026 SNT Soft Co., Ltd.
// SPDX-License-Identifier: BUSL-1.1

package store

// 네트워크 파일시스템의 이름표. 플랫폼마다 다르지만 판정은 여기 한곳에 두어 어느 기계에서나 잰다.

// remoteLinuxFS: statfs의 f_type이 네트워크 파일시스템이면 그 이름, 아니면 빈 문자열.
func remoteLinuxFS(magic uint32) string {
	switch magic {
	case 0x6969:
		return "NFS"
	case 0xFF534D42:
		return "CIFS"
	case 0xFE534D42:
		return "SMB2"
	case 0x517B:
		return "SMB"
	}
	return ""
}

// remoteDarwinFS: statfs의 f_fstypename이 네트워크 파일시스템이면 그 이름, 아니면 빈 문자열.
func remoteDarwinFS(name string) string {
	switch name {
	case "nfs":
		return "NFS"
	case "smbfs":
		return "SMB"
	case "afpfs":
		return "AFP"
	case "webdav":
		return "WebDAV"
	}
	return ""
}
