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

func FsType(path string) string {
	// Types of file systems that may be returned by `statfs`.
	// See https://github.com/torvalds/linux/blob/master/include/uapi/linux/magic.h
	fsTypes := map[int64]string{
		0xadf5:     "ADFS_SUPER_MAGIC",
		0xadff:     "AFFS_SUPER_MAGIC",
		0x5346414F: "AFS_SUPER_MAGIC",
		0x0187:     "AUTOFS_SUPER_MAGIC",
		0x00c36400: "CEPH_SUPER_MAGIC",
		0x73757245: "CODA_SUPER_MAGIC",
		0x28cd3d45: "CRAMFS_MAGIC",
		0x453dcd28: "CRAMFS_MAGIC_WEND",
		0x64626720: "DEBUGFS_MAGIC",
		0x73636673: "SECURITYFS_MAGIC",
		0xf97cff8c: "SELINUX_MAGIC",
		0x43415d53: "SMACK_MAGIC",
		0x858458f6: "RAMFS_MAGIC",
		0x01021994: "TMPFS_MAGIC",
		0x958458f6: "HUGETLBFS_MAGIC",
		0x73717368: "SQUASHFS_MAGIC",
		0xf15f:     "ECRYPTFS_SUPER_MAGIC",
		0x414A53:   "EFS_SUPER_MAGIC",
		0xE0F5E1E2: "EROFS_SUPER_MAGIC_V1",
		0xabba1974: "XENFS_SUPER_MAGIC",
		0xEF53:     "EXT4_SUPER_MAGIC", /* May also be EXT2_SUPER_MAGIC or EXT3_SUPER_MAGIC. */
		0x9123683E: "BTRFS_SUPER_MAGIC",
		0x3434:     "NILFS_SUPER_MAGIC",
		0xF2F52010: "F2FS_SUPER_MAGIC",
		0xf995e849: "HPFS_SUPER_MAGIC",
		0x9660:     "ISOFS_SUPER_MAGIC",
		0x72b6:     "JFFS2_SUPER_MAGIC",
		0x58465342: "XFS_SUPER_MAGIC",
		0x6165676C: "PSTOREFS_MAGIC",
		0xde5e81e4: "EFIVARFS_MAGIC",
		0x00c0ffee: "HOSTFS_SUPER_MAGIC",
		0x794c7630: "OVERLAYFS_SUPER_MAGIC",
		0x65735546: "FUSE_SUPER_MAGIC",
		0xca451a4e: "BCACHEFS_SUPER_MAGIC",
		0x137F:     "MINIX_SUPER_MAGIC",
		0x138F:     "MINIX_SUPER_MAGIC2",
		0x2468:     "MINIX2_SUPER_MAGIC",
		0x2478:     "MINIX2_SUPER_MAGIC2",
		0x4d5a:     "MINIX3_SUPER_MAGIC",
		0x4d44:     "MSDOS_SUPER_MAGIC",
		0x2011BAB0: "EXFAT_SUPER_MAGIC",
		0x564c:     "NCP_SUPER_MAGIC",
		0x6969:     "NFS_SUPER_MAGIC",
		0x7461636f: "OCFS2_SUPER_MAGIC",
		0x9fa1:     "OPENPROM_SUPER_MAGIC",
		0x002f:     "QNX4_SUPER_MAGIC",
		0x68191122: "QNX6_SUPER_MAGIC",
		0x6B414653: "AFS_FS_MAGIC",
		0x52654973: "REISERFS_SUPER_MAGIC",
		0x517B:     "SMB_SUPER_MAGIC",
		0xFF534D42: "CIFS_SUPER_MAGIC",
		0xFE534D42: "SMB2_SUPER_MAGIC",
		0x27e0eb:   "CGROUP_SUPER_MAGIC",
		0x63677270: "CGROUP2_SUPER_MAGIC",
		0x7655821:  "RDTGROUP_SUPER_MAGIC",
		0x57AC6E9D: "STACK_END_MAGIC",
		0x74726163: "TRACEFS_MAGIC",
		0x01021997: "V9FS_MAGIC",
		0x62646576: "BDEVFS_MAGIC",
		0x64646178: "DAXFS_MAGIC",
		0x42494e4d: "BINFMTFS_MAGIC",
		0x1cd1:     "DEVPTS_SUPER_MAGIC",
		0x6c6f6f70: "BINDERFS_SUPER_MAGIC",
		0xBAD1DEA:  "FUTEXFS_SUPER_MAGIC",
		0x50495045: "PIPEFS_MAGIC",
		0x9fa0:     "PROC_SUPER_MAGIC",
		0x534F434B: "SOCKFS_MAGIC",
		0x62656572: "SYSFS_MAGIC",
		0x9fa2:     "USBDEVICE_SUPER_MAGIC",
		0x11307854: "MTD_INODE_FS_MAGIC",
		0x09041934: "ANON_INODE_FS_MAGIC",
		0x73727279: "BTRFS_TEST_MAGIC",
		0x6e736673: "NSFS_MAGIC",
		0xcafe4a11: "BPF_FS_MAGIC",
		0x5a3c69f0: "AAFS_MAGIC",
		0x5a4f4653: "ZONEFS_MAGIC",
		0x15013346: "UDF_SUPER_MAGIC",
		0x444d4142: "DMA_BUF_MAGIC",
		0x454d444d: "DEVMEM_MAGIC",
		0x5345434d: "SECRETMEM_MAGIC",
		0x50494446: "PID_FS_MAGIC",
		0x474d454d: "GUEST_MEMFD_MAGIC",
		0x4E554C4C: "NULL_FS_MAGIC",

		/* The following mappings are not listed in `magic.h`. */
		0x42465331: "BEFS_SUPER_MAGIC",
		0x1BADFACE: "BFS_MAGIC",
		0x012FF7B7: "COH_SUPER_MAGIC",
		0x1373:     "DEVFS_SUPER_MAGIC",
		0x137D:     "EXT_SUPER_MAGIC",
		0xEF51:     "EXT2_OLD_SUPER_MAGIC",
		0x4244:     "HFS_SUPER_MAGIC",
		0x3153464a: "JFS_SUPER_MAGIC",
		0x5346544e: "NTFS_SB_MAGIC",
		0x7275:     "ROMFS_MAGIC",
		0x012FF7B6: "SYSV2_SUPER_MAGIC",
		0x012FF7B5: "SYSV4_SUPER_MAGIC",
		0x00011954: "UFS_MAGIC",
		0xa501FCF5: "VXFS_SUPER_MAGIC",
		0x012FF7B4: "XENIX_SUPER_MAGIC",
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

func FsSize(path string) uint64 {
	var fs syscall.Statfs_t
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return 0
	}
	//nolint:unconvert // Blocks is int64 on some operating systems (e.g. dragonfly).
	return uint64(fs.Bsize) * uint64(fs.Blocks)
}
