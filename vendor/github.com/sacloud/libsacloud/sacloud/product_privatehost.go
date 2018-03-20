package sacloud

// ProductPrivateHost 専有ホストプラン
type ProductPrivateHost struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propAvailability // 有功状態
	propCPU          // CPUコア数
	propMemoryMB     // メモリサイズ(MB単位)
	propClass        // クラス
	propServiceClass // サービスクラス
}
