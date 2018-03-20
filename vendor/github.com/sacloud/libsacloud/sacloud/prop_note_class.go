package sacloud

// ENoteClass スタートアップスクリプトクラス
type ENoteClass string

var (
	// NoteClassShell shellクラス
	NoteClassShell = ENoteClass("shell")
	// NoteClassYAMLCloudConfig yaml_cloud_configクラス
	NoteClassYAMLCloudConfig = ENoteClass("yaml_cloud_config")
)

// ENoteClasses 設定可能なスタートアップスクリプトクラス
var ENoteClasses = []ENoteClass{NoteClassShell, NoteClassYAMLCloudConfig}

// propNoteClass スタートアップスクリプトクラス情報内包型
type propNoteClass struct {
	Class ENoteClass `json:",omitempty"` // クラス
}

// GetClass クラス 取得
func (p *propNoteClass) GetClass() ENoteClass {
	return p.Class
}

// SetClass クラス 設定
func (p *propNoteClass) SetClass(c ENoteClass) {
	p.Class = c
}

// GetClassStr クラス 取得(文字列)
func (p *propNoteClass) GetClassStr() string {
	return string(p.Class)
}

// SetClassByStr クラス 設定(文字列)
func (p *propNoteClass) SetClassByStr(c string) {
	p.Class = ENoteClass(c)
}
