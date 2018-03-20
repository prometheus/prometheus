package sacloud

import "time"

// Bill 請求情報
type Bill struct {
	Amount         int64      `json:",omitempty"` // 金額
	BillID         int64      `json:",omitempty"` // 請求ID
	Date           *time.Time `json:",omitempty"` // 請求日
	MemberID       string     `json:",omitempty"` // 会員ID
	Paid           bool       `json:",omitempty"` // 支払済フラグ
	PayLimit       *time.Time `json:",omitempty"` // 支払い期限
	PaymentClassID int        `json:",omitempty"` // 支払いクラスID

}

// BillDetail 支払い明細情報
type BillDetail struct {
	Amount         int64      `json:",omitempty"` // 金額
	ContractID     int64      `json:",omitempty"` // 契約ID
	Description    string     `json:",omitempty"` // 説明
	Index          int        `json:",omitempty"` // インデックス
	ServiceClassID int64      `json:",omitempty"` // サービスクラスID
	Usage          int64      `json:",omitempty"` // 秒数
	Zone           string     `json:",omitempty"` // ゾーン
	ContractEndAt  *time.Time `json:",omitempty"` // 契約終了日時
}

// IsContractEnded 支払済か判定
func (d *BillDetail) IsContractEnded(t time.Time) bool {
	return d.ContractEndAt != nil && d.ContractEndAt.Before(t)
}
