// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows && !openbsd && !netbsd && !solaris && !386

package runtime

import (
	"strconv"
	"syscall"
)

// Statfs returns the file system type (Unix only).
func Statfs(path string) string {
	// Types of file systems that may be returned by `statfs`
	fsTypes := map[int64]string{
		0xadf5:     "ADFS_SUPER_MAGIC",
		0xADFF:     "AFFS_SUPER_MAGIC",
		0x42465331: "BEFS_SUPER_MAGIC",
		0x1BADFACE: "BFS_MAGIC",
		0xFF534D42: "CIFS_MAGIC_NUMBER",
		0x73757245: "CODA_SUPER_MAGIC",
		0x012FF7B7: "COH_SUPER_MAGIC",
		0x28cd3d45: "CRAMFS_MAGIC",
		0x1373:     "DEVFS_SUPER_MAGIC",
		0x00414A53: "EFS_SUPER_MAGIC",
		0x137D:     "EXT_SUPER_MAGIC",
		0xEF51:     "EXT2_OLD_SUPER_MAGIC",
		0xEF53:     "EXT4_SUPER_MAGIC",
		0x4244:     "HFS_SUPER_MAGIC",
		0xF995E849: "HPFS_SUPER_MAGIC",
		0x958458f6: "HUGETLBFS_MAGIC",
		0x9660:     "ISOFS_SUPER_MAGIC",
		0x72b6:     "JFFS2_SUPER_MAGIC",
		0x3153464a: "JFS_SUPER_MAGIC",
		0x137F:     "MINIX_SUPER_MAGIC",
		0x138F:     "MINIX_SUPER_MAGIC2",
		0x2468:     "MINIX2_SUPER_MAGIC",
		0x2478:     "MINIX2_SUPER_MAGIC2",
		0x4d44:     "MSDOS_SUPER_MAGIC",
		0x564c:     "NCP_SUPER_MAGIC",
		0x6969:     "NFS_SUPER_MAGIC",
		0x5346544e: "NTFS_SB_MAGIC",
		0x9fa1:     "OPENPROM_SUPER_MAGIC",
		0x9fa0:     "PROC_SUPER_MAGIC",
		0x002f:     "QNX4_SUPER_MAGIC",
		0x52654973: "REISERFS_SUPER_MAGIC",
		0x7275:     "ROMFS_MAGIC",
		0x517B:     "SMB_SUPER_MAGIC",
		0x012FF7B6: "SYSV2_SUPER_MAGIC",
		0x012FF7B5: "SYSV4_SUPER_MAGIC",
		0x01021994: "TMPFS_MAGIC",
		0x15013346: "UDF_SUPER_MAGIC",
		0x00011954: "UFS_MAGIC",
		0x9fa2:     "USBDEVICE_SUPER_MAGIC",
		0xa501FCF5: "VXFS_SUPER_MAGIC",
		0x012FF7B4: "XENIX_SUPER_MAGIC",
		0x58465342: "XFS_SUPER_MAGIC",
		0x012FD16D: "_XIAFS_SUPER_MAGIC",
	}

	var fs syscall.Statfs_t
	err := syscall.Statfs(path, &fs)
	// nolintlint might cry out depending on the architecture (e.g. ARM64), so ignore it.
	//nolint:unconvert,nolintlint // This ensures Type format on all Platforms.
	localType := int64(fs.Type)
	if err != nil {
		return strconv.FormatInt(localType, 16)
	}
	if fsType, ok := fsTypes[localType]; ok {
		return fsType
	}
	return strconv.FormatInt(localType, 16)
}
