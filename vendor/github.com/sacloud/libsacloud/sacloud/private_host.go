package sacloud

// PrivateHost 専有ホスト
type PrivateHost struct {
	*Resource       // ID
	propName        // 名称
	propDescription // 説明

	propPrivateHostPlan  // 専有ホストプラン
	propHost             // ホスト(物理)
	propAssignedCPU      // 割当済みCPUコア数
	propAssignedMemoryMB // 割当済みメモリ(MB)

	propIcon      // アイコン
	propTags      // タグ
	propCreatedAt // 作成日時

}
