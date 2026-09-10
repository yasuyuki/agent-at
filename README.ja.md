# agent-at

インストール・認証済みのエージェントをローカル時刻に一度起動するタイマーです。
タイマー自体に外部ランタイムは不要です。主対象は Windows 10／11 x64。
ソースは Linux／macOS でも利用でき、対話型は現在のターミナルで動作します。
[English](README.md)

**v0.2.0** — Codex／Claude Code の予約、会話再開、起動元ターミナルの継承に対応。
[Windows x64 ZIP](https://github.com/yasuyuki/agent-at/releases/download/v0.2.0/agent-at-windows-x64.zip)と
[チェックサム](https://github.com/yasuyuki/agent-at/releases/download/v0.2.0/SHA256SUMS)は
[リリースページ](https://github.com/yasuyuki/agent-at/releases/tag/v0.2.0)から取得できます。
ZIP を展開して `dist/agent-at.exe` を使用します。旧 `codex-at` プレビューはリリース履歴に残しています。

旧 Codex 版の Windows 11 ネイティブ自動試験と基本対話5手順は確認済みです。ヘッドレスでは、
無害なシェル操作1件の自動レビュー・許可も確認しました。2026-09-10 に本人が Windows 10 の
ジョブ実行と保存ファイルの回収を確認しましたが、日本語表示の文字化けが報告されました。実スリープ復帰、
Ctrl+C、自動クローズなどは未検証です。確認範囲と残項目は
[検証記録](docs/VERIFICATION.md)に明記しています。
Claude のヘッドレス予約実行と同一会話の再開は Linux の実 CLI で確認済みです。
Claude の対話型と Windows 実画面の動作は未確認です。

## 使い方

現在のプロジェクトのディレクトリを PowerShell で開き、実行します。

```powershell
.\dist\agent-at.exe --at 23:30 -- "現在の変更をレビューしてください"
```

長文・改行を含む依頼は UTF-8 ファイルへ保存します。

```powershell
$timerArgs = @(
  '--agent', 'claude'
  '--at', '23:30:15'
  '--cd', 'C:\Projects\my project'
  '--prompt-file', '.\request.txt'
)
.\dist\agent-at.exe @timerArgs
```

| 入力 | 動作 |
| --- | --- |
| `--at TIME` | 新規依頼では必須、再開では省略可。`HH:mm[:ss]` または `YYYY-MM-DDTHH:mm[:ss]` |
| 位置引数／`--prompt-file FILE` | 新規依頼でどちらか一方。再開との併用不可。位置引数は全オプションの後に1つ |
| `--cd DIR` | 作業場所。省略時は予約を始めたディレクトリ |
| `--add-dir DIR` | 追加ディレクトリ。複数指定可 |
| `--agent codex\|claude` | 起動するエージェント。既定は `codex` |
| `--model MODEL` | 指定時だけ渡す。省略時は選択したエージェントの設定を継承 |
| `--agent-path PATH` | 起動先。省略時は予約時に選択したエージェントを PATH から解決 |
| `--no-auto-approve` | エージェント既定の自動承認指定を省略し、設定を継承 |
| `--headless` | ヘッドレス実行。依頼文を標準入力へ渡す（Claude は `--print`） |
| `--resume ID` | 保存済み会話を再開し、`resume` だけ送信。時刻省略なら即時 |
| `--new-console` | Windows で別窓を開く。既定は起動元ターミナルを継承 |
| `--close-on-exit` | Windows の専用コンソールをエージェント終了後に閉じる |
| `--help` | 英語ヘルプ |

旧プレビューから移行する場合は、`--codex` の代わりに `--agent-path`、
`--no-approve-for-me` の代わりに `--no-auto-approve` を使います。旧名は別名ではありません。
リポジトリは [yasuyuki/agent-at](https://github.com/yasuyuki/agent-at) です。
現在のバイナリと Go module も `agent-at` に統一しています。

利用枠制限で止まったジョブは、選択したエージェントが表示した完全な会話 ID（hash）を指定して再開します。
保存時と同じアカウント・選択したエージェントの home を使用してください。

```powershell
$sessionId = Read-Host 'Agent session ID'
.\dist\agent-at.exe --resume $sessionId
```

`--at 23:30` を加えれば枠の回復時刻に予約でき、`--headless` も併用できます。
会話へ送る文は常に `resume` だけです。`.cmd` でもファイル読取指示に変換しません。Claude の再開例です。

```powershell
$sessionId = Read-Host 'Claude session ID'
.\dist\agent-at.exe --agent claude --resume $sessionId
```

独自のジョブ台帳は持たず、ID を選択したエージェントの再開機能へ渡します。失われた会話履歴の復元や
利用枠制限の回避はしません。`--cd` の既定は通常どおり起動した場所なので、元のプロジェクトで
実行するか、そのディレクトリを明示してください。Claude では `--cd` がプロセスの作業ディレクトリになります。

時刻だけなら次の到来時刻へ予約します。同時刻・過去の時刻は翌日です。
日時指定は未来だけを受理します。不正値、夏時間などで存在しない・二通りある
ローカル日時を拒否します。予約時に実行時点、パス、起動先、依頼文を確定します。
後でタイムゾーンを変えても予約を解釈し直しません。
依頼文ファイルは UTF-8（BOM 可）。空白だけ、UTF-8 不正、NUL、読めないファイル、
存在しないディレクトリを待機前に拒否します。Windows の `.cmd` 経由ではパスとモデル名に改行・二重引用符を使えません。

タイマーを開いたままにしてください。待機中の Ctrl+C で取消（終了コード130）。
スリープを解除する機能はありません。予定時刻後の復帰時は通常1秒以内に一度起動します。
システム時計の変更は実行タイミングへ反映します。プロセス終了・再起動で予約は失われます。
複数予約は独立し、永続化、繰り返し、再試行はありません。

## エージェントとウィンドウの寿命

選択したエージェントを実行アカウントでインストールし、認証を済ませておく必要があります。
選択したエージェント自体のランタイム・ネットワーク要件は残ります。npm の `.cmd` は Node.js が
必要な場合があります。[公式 CLI リファレンス](https://developers.openai.com/codex/cli/reference)
も参照してください。[Claude Code CLI リファレンス](https://code.claude.com/docs/en/cli-reference)も参照してください。
Codex の既定は `--approve-for-me`、Claude の既定は `--permission-mode auto` です。
`--no-auto-approve` はこれらの既定指定を省略し、設定を継承します。自動的に別の承認・sandbox 設定へ変更する回避策はありません。

対話型は全 OS で起動元のターミナルを使います。Codex には `--no-alt-screen` を渡して履歴を残します。
Windows でも PowerShell／cmd から起動したコンソールや Windows Terminal のタブを継承します。
親プロセスからシェルを推測して起動し直す処理はありません。作業用シェルは
選択したエージェントの設定に従います。最初の依頼が完了しても対話を続けられ、タイマーはエージェントの
終了を待って終了コードを返します。Claude の対話型は初期依頼を引数で受け取り、ヘッドレスは
`--print` で標準入力を読みます（`-` は渡しません）。Claude の表示は既定の描画方式 を使い、
スクロールバック保持を保証しません。

Windows のコンソール接続時は、エージェント起動前に入出力コードページを UTF-8 に設定し、
終了後に元へ戻します。リダイレクトされたストリームは変更しません。従来のコードページとの
不一致に対処するもので、日本語グリフの表示には対応フォントも必要です。

Windows で従来の別窓を使う場合は `--new-console` を指定します。
選択したエージェントの終了後はキー待ちを行い、`--close-on-exit` なら待たずに閉じます。
この場合だけ、親タイマーは子のプロセス生成結果を受け取って終了します。
親の終了コード0は認証や依頼完了の成功を意味しません。`.cmd` ではインタープリターの
生成結果です。後続のエラーは専用コンソールで確認します。

ヘッドレスは標準出力・標準エラー・終了コードを呼出元へ返します。
`--new-console` はヘッドレスと Linux／macOS では効果がありません。
`--close-on-exit` は Windows の別窓だけに作用し、起動元のターミナルは閉じません。
入力エラーは2、起動エラーは1です。

Windows の `.exe` は直接、信頼できる `.cmd` はシステムの `cmd.exe` で起動します。
遅延展開を無効化し、引用した環境変数経由で引数を渡します。shim は標準的な npm の
`%*` 転送のように、`CALL` や追加のシェル再評価なしで引数を渡す必要があります。
他の Windows スクリプト形式は対象外です。生成 launcher や private な管理コードは使いません。

新規依頼の対話型 `.cmd` 経由と Windows のコマンド行上限に収まらない長文は、システム一時領域の
専用 `agent-at-*` ディレクトリへ `prompt.txt` として保存します。元の UTF-8 内容を保ち、
選択したエージェントにはファイルを読んで実行する依頼を渡し、その場所を `--add-dir` に加えます。
ネイティブな添付機能ではなくファイル読取の指示なので、モデルが従うかは別途実機確認が必要です。
この追加先は選択したエージェントの追加ディレクトリ設定により書込可能になります。
Windows では作成時に実行ユーザーだけの保護 DACL を指定し、親 TEMP の追加権限を継承しません。
エージェント終了後、キー待ちの前に削除します。通常の起動失敗時も削除します。
ヘッドレスは常に標準入力を使います。強制終了・OS 終了・他プロセスのファイル保持で
一時領域に依頼文が残る場合があります。稼働中のセッションのものは削除しないでください。

## ビルド・検証

[Go 公式配布元](https://go.dev/dl/)を使います。2026-09-08 に公式 API が返した最新安定版
Go 1.27.1 を、配布 SHA-256 と照合して使用しました。C コンパイラー・外部 Go 依存は不要。
タイムゾーンデータは組み込みます。署名なし Windows x64 成果物は `dist/agent-at.exe`、
ハッシュは `dist/SHA256SUMS` です。

Windows のリポジトリルートで PowerShell から実行します。

```powershell
go test ./...
go vet ./...
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -buildvcs=false -o dist/agent-at.exe .
```

Linux／macOS のリポジトリルートでは次を使えます。

```sh
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o agent-at .
```

クロスビルドの手順は [英語 README](README.md#build-and-test) にあります。
Windows 用テストのコンパイルだけでは実行済みになりません。
[検証記録の Windows 確認項目](docs/VERIFICATION.md)を実機で完了させてください。
自動テストのエージェントは偽物であり、認証や AI サービスへの通信はしません。

MIT ライセンス。exe に含む Go runtime・標準ライブラリの
[ライセンス表示](docs/GO-LICENSE.txt)も配布物に同梱します。
OpenAI／Anthropic の公式製品ではありません。
