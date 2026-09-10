# codex-at

インストール・認証済みの Codex をローカル時刻に一度起動するタイマーです。
タイマー自体に外部ランタイムは不要です。主対象は Windows 10／11 x64。
ソースは Linux／macOS でも利用でき、対話型は現在のターミナルで動作します。
[English](README.md)

**v0.1.0-preview.1 — 初回プレビュー版。**
[Windows x64 ZIP](https://github.com/yasuyuki/codex-at/releases/download/v0.1.0-preview.1/codex-at-windows-x64.zip)と
[チェックサム](https://github.com/yasuyuki/codex-at/releases/download/v0.1.0-preview.1/SHA256SUMS)は
[リリースページ](https://github.com/yasuyuki/codex-at/releases/tag/v0.1.0-preview.1)から取得できます。
ZIP は展開して使用します。以下の `--resume`、起動元ターミナルの継承、
`--new-console`、UTF-8 対策は現在のソース・ビルドの機能であり、初回リリース ZIP には含まれません。

Windows 11 のネイティブ自動試験と基本対話5手順は確認済みです。ヘッドレスでは、
無害なシェル操作1件の自動レビュー・許可も確認しました。2026-09-10 に本人が Windows 10 の
ジョブ実行と保存ファイルの回収を確認しましたが、日本語表示の文字化けが報告されました。実スリープ復帰、
Ctrl+C、自動クローズなどは未検証です。確認範囲と残項目は
[検証記録](docs/VERIFICATION.md)に明記しています。

## 使い方

展開したプロジェクトのディレクトリを PowerShell で開き、実行します。

```powershell
.\dist\codex-at.exe --at 23:30 -- "現在の変更をレビューしてください"
```

長文・改行を含む依頼は UTF-8 ファイルへ保存します。

```powershell
$timerArgs = @(
  '--at', '23:30:15'
  '--cd', 'C:\Projects\my project'
  '--prompt-file', '.\request.txt'
)
.\dist\codex-at.exe @timerArgs
```

| 入力 | 動作 |
| --- | --- |
| `--at TIME` | 新規依頼では必須、再開では省略可。`HH:mm[:ss]` または `YYYY-MM-DDTHH:mm[:ss]` |
| 位置引数／`--prompt-file FILE` | 新規依頼でどちらか一方。再開との併用不可。位置引数は全オプションの後に1つ |
| `--cd DIR` | 作業場所。省略時は予約を始めたディレクトリ |
| `--add-dir DIR` | 追加ディレクトリ。複数指定可 |
| `--model MODEL` | 指定時だけ渡す。省略時は Codex の設定を継承 |
| `--codex PATH` | 起動先。省略時は予約時に PATH の `codex` を解決 |
| `--no-approve-for-me` | 既定の `--approve-for-me` を省略 |
| `--headless` | `codex exec`。依頼文を標準入力へ渡す |
| `--resume ID` | Codex の保存済み会話を再開し、`resume` だけ送信。時刻省略なら即時 |
| `--new-console` | Windows で別窓を開く。既定は起動元ターミナルを継承 |
| `--close-on-exit` | Windows の専用コンソールを Codex 終了後に閉じる |
| `--help` | 英語ヘルプ |

利用枠制限で止まったジョブは、Codex が表示した完全な会話 ID（hash）を指定して再開します。
保存時と同じアカウント・Codex home を使用してください。

```powershell
$sessionId = Read-Host 'Codex session ID'
.\dist\codex-at.exe --resume $sessionId
```

`--at 23:30` を加えれば枠の回復時刻に予約でき、`--headless` も併用できます。
会話へ送る文は常に `resume` だけです。`.cmd` でもファイル読取指示に変換しません。
独自のジョブ台帳は持たず、ID を Codex の再開機能へ渡します。失われた会話履歴の復元や
利用枠制限の回避はしません。`--cd` の既定は通常どおり起動した場所なので、元のプロジェクトで
実行するか、そのディレクトリを明示してください。

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

## Codex とウィンドウの寿命

実行アカウントで Codex のインストールと認証を済ませておく必要があります。
Codex 自体のランタイム・ネットワーク要件は残ります。npm の `.cmd` は Node.js が
必要な場合があります。[公式 CLI リファレンス](https://developers.openai.com/codex/cli/reference)
も参照してください。調査した Codex 0.153.4 のヘルプでは対話型と `exec` の両方に
`--approve-for-me` がありました。引数の受理は自動レビューの実動作の証明ではありません。
バージョン・モデル・アカウントごとの実測が必要です。未対応引数でも別の承認・sandbox
設定へ自動変更しません。モデルや認証・設定は継承しますが、既定の承認フラグは自動レビューを
要求します。渡したくない場合は `--no-approve-for-me` を指定します。

対話型は全 OS で起動元のターミナルを使い、`--no-alt-screen` で履歴を残します。
Windows でも PowerShell／cmd から起動したコンソールや Windows Terminal のタブを継承します。
親プロセスからシェルを推測して起動し直す処理はありません。Codex の作業用シェルは
Codex 自身の設定に従います。最初の依頼が完了しても対話を続けられ、タイマーは Codex の
終了を待って終了コードを返します。

Windows のコンソール接続時は、Codex 起動前に入出力コードページを UTF-8 に設定し、
終了後に元へ戻します。リダイレクトされたストリームは変更しません。従来のコードページとの
不一致に対処するもので、日本語グリフの表示には対応フォントも必要です。

Windows で従来の別窓を使う場合は `--new-console` を指定します。
Codex 終了後はキー待ちを行い、`--close-on-exit` なら待たずに閉じます。
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
専用 `codex-at-*` ディレクトリへ `prompt.txt` として保存します。元の UTF-8 内容を保ち、
Codex にはファイルを読んで実行する依頼を渡し、その場所を `--add-dir` に加えます。
ネイティブな添付機能ではなくファイル読取の指示なので、モデルが従うかは別途実機確認が必要です。
この追加先は Codex の追加ディレクトリ設定により書込可能になります。
Windows では作成時に実行ユーザーだけの保護 DACL を指定し、親 TEMP の追加権限を継承しません。
Codex 終了後、キー待ちの前に削除します。通常の起動失敗時も削除します。
ヘッドレスは常に標準入力を使います。強制終了・OS 終了・他プロセスのファイル保持で
一時領域に依頼文が残る場合があります。稼働中のセッションのものは削除しないでください。

## ビルド・検証

[Go 公式配布元](https://go.dev/dl/)を使います。2026-09-08 に公式 API が返した最新安定版
Go 1.27.1 を、配布 SHA-256 と照合して使用しました。C コンパイラー・外部 Go 依存は不要。
タイムゾーンデータは組み込みます。署名なし Windows x64 成果物は `dist/codex-at.exe`、
ハッシュは `dist/SHA256SUMS` です。

Windows のリポジトリルートで PowerShell から実行します。

```powershell
go test ./...
go vet ./...
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -trimpath -buildvcs=false -o dist/codex-at.exe .
```

Linux／macOS のリポジトリルートでは次を使えます。

```sh
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o codex-at .
```

クロスビルドの手順は [英語 README](README.md#build-and-test) にあります。
Windows 用テストのコンパイルだけでは実行済みになりません。
[検証記録の Windows 確認項目](docs/VERIFICATION.md)を実機で完了させてください。
自動テストの Codex は偽物であり、認証や AI サービスへの通信はしません。

MIT ライセンス。exe に含む Go runtime・標準ライブラリの
[ライセンス表示](docs/GO-LICENSE.txt)も配布物に同梱します。
OpenAI の公式製品ではありません。
