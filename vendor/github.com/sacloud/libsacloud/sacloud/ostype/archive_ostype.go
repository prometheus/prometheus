// Package ostype is define OS type of SakuraCloud public archive
package ostype

//go:generate stringer -type=ArchiveOSTypes

// ArchiveOSTypes パブリックアーカイブOS種別
type ArchiveOSTypes int

const (
	// CentOS OS種別:CentOS
	CentOS ArchiveOSTypes = iota
	// CentOS6 OS種別:CentOS6
	CentOS6
	// Ubuntu OS種別:Ubuntu
	Ubuntu
	// Debian OS種別:Debian
	Debian
	// VyOS OS種別:VyOS
	VyOS
	// CoreOS OS種別:CoreOS
	CoreOS
	// RancherOS OS種別:RancherOS
	RancherOS
	// Kusanagi OS種別:Kusanagi(CentOS)
	Kusanagi
	// SophosUTM OS種別:Sophos UTM
	SophosUTM
	// FreeBSD OS種別:FreeBSD
	FreeBSD
	// Windows2012 OS種別:Windows Server 2012 R2 Datacenter Edition
	Windows2012
	// Windows2012RDS OS種別:Windows Server 2012 R2 for RDS
	Windows2012RDS
	// Windows2012RDSOffice OS種別:Windows Server 2012 R2 for RDS(Office)
	Windows2012RDSOffice
	// Windows2016 OS種別:Windows Server 2016 Datacenter Edition
	Windows2016
	// Windows2016RDS OS種別:Windows Server 2016 RDS
	Windows2016RDS
	// Windows2016RDSOffice OS種別:Windows Server 2016 RDS(Office)
	Windows2016RDSOffice
	// Windows2016SQLServerWeb OS種別:Windows Server 2016 SQLServer(Web)
	Windows2016SQLServerWeb
	// Windows2016SQLServerStandard OS種別:Windows Server 2016 SQLServer(Standard)
	Windows2016SQLServerStandard
	// Windows2016SQLServerStandardAll OS種別:Windows Server 2016 SQLServer(Standard) + RDS + Office
	Windows2016SQLServerStandardAll
	// Custom OS種別:カスタム
	Custom
)

// OSTypeShortNames OSTypeとして利用できる文字列のリスト
var OSTypeShortNames = []string{
	"centos", "centos6", "ubuntu", "debian", "vyos", "coreos",
	"rancheros", "kusanagi", "sophos-utm", "freebsd",
	"windows2012", "windows2012-rds", "windows2012-rds-office",
	"windows2016", "windows2016-rds", "windows2016-rds-office",
	"windows2016-sql-web", "windows2016-sql-standard", "windows2016-sql-standard-all",
}

// IsWindows Windowsか
func (o ArchiveOSTypes) IsWindows() bool {
	switch o {
	case Windows2012, Windows2012RDS, Windows2012RDSOffice,
		Windows2016, Windows2016RDS, Windows2016RDSOffice,
		Windows2016SQLServerWeb, Windows2016SQLServerStandard, Windows2016SQLServerStandardAll:
		return true
	default:
		return false
	}
}

// IsSupportDiskEdit ディスクの修正機能をフルサポートしているか(Windowsは一部サポートのためfalseを返す)
func (o ArchiveOSTypes) IsSupportDiskEdit() bool {
	switch o {
	case CentOS, CentOS6, Ubuntu, Debian, VyOS, CoreOS, RancherOS, Kusanagi, FreeBSD:
		return true
	default:
		return false
	}
}

// StrToOSType 文字列からArchiveOSTypesへの変換
func StrToOSType(osType string) ArchiveOSTypes {
	switch osType {
	case "centos":
		return CentOS
	case "centos6":
		return CentOS6
	case "ubuntu":
		return Ubuntu
	case "debian":
		return Debian
	case "vyos":
		return VyOS
	case "coreos":
		return CoreOS
	case "rancheros":
		return RancherOS
	case "kusanagi":
		return Kusanagi
	case "sophos-utm":
		return SophosUTM
	case "freebsd":
		return FreeBSD
	case "windows2012":
		return Windows2012
	case "windows2012-rds":
		return Windows2012RDS
	case "windows2012-rds-office":
		return Windows2012RDSOffice
	case "windows2016":
		return Windows2016
	case "windows2016-rds":
		return Windows2016RDS
	case "windows2016-rds-office":
		return Windows2016RDSOffice
	case "windows2016-sql-web":
		return Windows2016SQLServerWeb
	case "windows2016-sql-standard":
		return Windows2016SQLServerStandard
	case "windows2016-sql-standard-all":
		return Windows2016SQLServerStandardAll
	default:
		return Custom
	}
}
