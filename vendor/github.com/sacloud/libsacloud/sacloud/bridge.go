package sacloud

import (
	"encoding/json"
)

// Bridge ブリッジ
type Bridge struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propServiceClass // サービスクラス
	propRegion       // リージョン
	propCreatedAt    // 作成日時

	Info *struct { // インフォ
		Switches []*struct { // 接続スイッチリスト
			*Switch             // スイッチ
			ID      json.Number `json:",omitempty"` // (HACK) ID
		}
	}

	SwitchInZone *struct { // ゾーン内接続スイッチ
		*Resource             // ID
		propScope             // スコープ
		Name           string `json:",omitempty"` // 名称
		ServerCount    int    `json:",omitempty"` // 接続サーバー数
		ApplianceCount int    `json:",omitempty"` // 接続アプライアンス数
	}
}
