package sacloud

import (
	"encoding/json"
	"fmt"
	"net"
)

// Switch スイッチ
type Switch struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propServiceClass // サービスクラス
	propIcon         // アイコン
	propTags         // タグ
	propCreatedAt    // 作成日時

	ServerCount    int            `json:",omitempty"` // 接続サーバー数
	ApplianceCount int            `json:",omitempty"` // 接続アプライアンス数
	Scope          EScope         `json:",omitempty"` // スコープ
	Subnet         *Subnet        `json:",omitempty"` // サブネット
	UserSubnet     *Subnet        `json:",omitempty"` // ユーザー定義サブネット
	Subnets        []SwitchSubnet `json:",omitempty"` // サブネット
	IPv6Nets       []IPv6Net      `json:",omitempty"` // IPv6サブネットリスト
	Internet       *Internet      `json:",omitempty"` // ルーター

	Bridge *struct { // 接続先ブリッジ(Info.Switches配下のIDデータ型HACK)
		*Bridge // ブリッジ
		Info    *struct {
			Switches []struct { // 接続スイッチリスト
				*Switch             // スイッチ
				ID      json.Number `json:",omitempty"` // HACK
			}
		}
	} `json:",omitempty"`

	//HybridConnection //REMARK: !!ハイブリッド接続 not support!!
}

// SwitchSubnet スイッチサブネット
type SwitchSubnet struct {
	*Subnet
	IPAddresses struct { // IPアドレス範囲
		Min string `json:",omitempty"` // IPアドレス開始
		Max string `json:",omitempty"` // IPアドレス終了
	}
}

// GetDefaultIPAddressesForVPCRouter VPCルーター接続用にサブネットからIPアドレスを3つ取得
func (s *Switch) GetDefaultIPAddressesForVPCRouter() (string, string, string, error) {

	if s.Subnets == nil || len(s.Subnets) < 1 {
		return "", "", "", fmt.Errorf("switch[%d].Subnets is nil", s.ID)
	}

	baseAddress := net.ParseIP(s.Subnets[0].IPAddresses.Min).To4()
	address1 := net.IPv4(baseAddress[0], baseAddress[1], baseAddress[2], baseAddress[3]+1)
	address2 := net.IPv4(baseAddress[0], baseAddress[1], baseAddress[2], baseAddress[3]+2)

	return baseAddress.String(), address1.String(), address2.String(), nil
}

// GetIPAddressList IPアドレス範囲内の全てのIPアドレスを取得
func (s *Switch) GetIPAddressList() ([]string, error) {
	if s.Subnets == nil || len(s.Subnets) < 1 {
		return nil, fmt.Errorf("switch[%d].Subnets is nil", s.ID)
	}

	//さくらのクラウドの仕様上/24までしか割り当てできないためこのロジックでOK
	baseIP := net.ParseIP(s.Subnets[0].IPAddresses.Min).To4()
	min := baseIP[3]
	max := net.ParseIP(s.Subnets[0].IPAddresses.Max).To4()[3]

	var i byte
	ret := []string{}
	for (min + i) <= max { //境界含む
		ip := net.IPv4(baseIP[0], baseIP[1], baseIP[2], baseIP[3]+i)
		ret = append(ret, ip.String())
		i++
	}

	return ret, nil
}
