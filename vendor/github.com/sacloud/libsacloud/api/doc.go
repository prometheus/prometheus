// Package api is the Low Level APIs for maganing resources on SakuraCloud.
//
// さくらのクラウドでのリソース操作用の低レベルAPIです。
// さくらのクラウドAPI呼び出しを行います。
//
// Basic usage
//
// はじめにAPIクライアントを作成します。以降は作成したAPIクライアントを通じて各操作を行います。
//
// APIクライアントの作成にはAPIトークン、APIシークレット、対象ゾーン情報が必要です。
//
// あらかじめ、さくらのクラウド コントロールパネルにてAPIキーを発行しておいてください。
//
//	token := "PUT YOUR TOKEN"
//	secret := "PUT YOUR SECRET"
//	zone := "tk1a"
//
//	// クライアントの作成
//	client := api.NewClient(token, secret, zone)
//
// Select target resource
//
// 操作対象のリソースごとにAPIクライアントのフィールドを用意しています。
// 例えばサーバーの操作を行う場合はAPIクライアントの"Server"フィールドを通じて操作します。
//
// フィールドの一覧は[type API]のドキュメントを参照してください。
//
//	// サーバーの検索の場合
//	client.Server.Find()
//
//	// ディスクの検索の場合
//	client.Disk.Find()
//
//
// Find resource
//
// リソースの検索を行うには、条件を指定してFind()を呼び出します。
//
// APIクライアントでは、検索条件の指定にFluent APIを採用しているため、メソッドチェーンで条件を指定できます。
//
//	// サーバーの検索
//	res, err := client.Server.
//		WithNameLike("server name"). // サーバー名に"server name"が含まれる
//		Offset(0).                   // 検索結果の位置0(先頭)から取得
//		Limit(5).                    // 5件取得
//		Include("Name").             // 結果にName列を含める
//		Include("Description").      // 結果にDescription列を含める
//		Find()                       // 検索実施
//
//	if err != nil {
//		panic(err)
//	}
//
//	fmt.Printf("response: %#v", res.Servers)
//
// Create resource
//
// 新規作成用パラメーターを作成し、値を設定します。
// その後APIクライアントのCreate()を呼び出します。
//
//	// スイッチの作成
//	param := client.Switch.New()           // 新規作成用パラメーターの作成
//	param.Name = "example"                 // 値の設定(名前)
//	param.Description = "example"          // 値の設定(説明)
//	sw, err := client.Switch.Create(param) // 作成
//
//	if err != nil {
//		panic(err)
//	}
//
//	fmt.Printf("Created switch: %#v", sw)
//
// リソースの作成は非同期で行われます。
//
// このため、サーバーやディスクなどの作成に時間がかかるリソースに対して
// Create()呼び出し直後に操作を行おうとした場合にエラーとなることがあります。
//
// 必要に応じて適切なメソッドを呼び出し、リソースが適切な状態になるまで待機してください。
//
//	// パブリックアーカイブからディスク作成
//	archive, _ := client.Archive.FindLatestStableCentOS()
//	// ディスクの作成
//	param := client.Disk.New()
//	param.Name = "example"                 // 値の設定(名前)
//	param.SetSourceArchive(archive.ID)      // コピー元にCentOSパブリックアーカイブを指定
//	disk, err := client.Disk.Create(param) // 作成
//
//	if err != nil {
//		panic(err)
//	}
//
//	// 作成完了まで待機
//	err = client.Disk.SleepWhileCopying(disk.ID, client.DefaultTimeoutDuration)
//
//	// タイムアウト発生の場合errが返る
//	if err != nil {
//		panic(err)
//	}
//
//	fmt.Printf("Created disk: %#v", disk)
//
// Update resource
//
// APIクライアントのUpdate()メソッドを呼び出します。
//
//	req, err := client.Server.Find()
//	if err != nil {
//		panic(err)
//	}
//	server := &req.Servers[0]
//
//	// 更新
//	server.Name = "update"                                        // サーバー名を変更
//	server.AppendTag("example-tag")                               // タグを追加
//	updatedServer, err := client.Server.Update(server.ID, server) //更新実行
//	if err != nil {
//		panic(err)
//	}
//	fmt.Printf("Updated server: %#v", updatedServer)
//
// 更新内容によってはあらかじめシャットダウンなどのリソースの操作が必要になる場合があります。
//
// 詳細はさくらのクラウド ドキュメントを参照下さい。
// http://cloud.sakura.ad.jp/document/
//
// Delete resource
//
// APIクライアントのDeleteメソッドを呼び出します。
//
//	// 削除
//	deletedSwitch, err := client.Switch.Delete(sw.ID)
//	if err != nil {
//		panic(err)
//	}
//	fmt.Printf("Deleted switch: %#v", deletedSwitch)
//
// 対象リソースによってはあらかじめシャットダウンや切断などの操作が必要になる場合があります。
//
// 詳細はさくらのクラウド ドキュメントを参照下さい。
// http://cloud.sakura.ad.jp/document/
//
// PowerManagement
//
// サーバーやアプライアンスなどの電源操作もAPIを通じて行えます。
//
//	// 起動
//	_, err = client.Server.Boot(server.ID)
//	if err != nil {
//		panic(err)
//	}
//
//	// 起動完了まで待機
//	err = client.Server.SleepUntilUp(server.ID, client.DefaultTimeoutDuration)
//
//	// シャットダウン
//	_, err = client.Server.Shutdown(server.ID) // gracefulシャットダウン
//
//	if err != nil {
//		panic(err)
//	}
//
//	// ダウンまで待機
//	err = client.Server.SleepUntilDown(server.ID, client.DefaultTimeoutDuration)
//
// 電源関連のAPI呼び出しは非同期で行われます。
//
// 必要に応じて適切なメソッドを呼び出し、リソースが適切な状態になるまで待機してください。
//
// Other
//
// APIクライアントでは開発時のデバッグ出力が可能です。
// 以下のようにTraceModeフラグをtrueに設定した上でAPI呼び出しを行うと、標準出力へトレースログが出力されます。
//
//	client.TraceMode = true
//
package api
