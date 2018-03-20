package api

// NoteAPI スタートアップスクリプトAPI
type NoteAPI struct {
	*baseAPI
}

// NewNoteAPI スタートアップスクリプトAPI作成
func NewNoteAPI(client *Client) *NoteAPI {
	return &NoteAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "note"
			},
		},
	}
}
