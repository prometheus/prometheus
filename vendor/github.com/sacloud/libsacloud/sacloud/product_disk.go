package sacloud

// ProductDisk ディスクプラン
type ProductDisk struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propStorageClass // ストレージクラス
	propAvailability // 有功状態

	Size []struct { // サイズ
		propAvailability // 有功状態
		propSizeMB       // サイズ(MB単位)
		propServiceClass // サービスクラス

		DisplaySize   int    `json:",omitempty"` // 表示サイズ
		DisplaySuffix string `json:",omitempty"` // 表示サフィックス

	} `json:",omitempty"`
}
