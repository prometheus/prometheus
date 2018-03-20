package sacloud

// ProductServer サーバープラン
type ProductServer struct {
	*Resource        // ID
	propName         // 名称
	propDescription  // 説明
	propAvailability // 有功状態
	propCPU          // CPUコア数
	propMemoryMB     // メモリサイズ(MB単位)
	propServiceClass // サービスクラス
}
