# agent-at

## 端末を閉じても予約を保持する（未リリース・Windows専用）

[Issue #3](https://github.com/yasuyuki/agent-at/issues/3) の候補実装で
`--persist`／`--list`／`--remove` を追加しています。未統合のwake実装に依存し、
v0.2.0配布版には含まれません。初回のWindows native受入は不合格で、修正候補の再受入待ちです。
[検証記録](docs/VERIFICATION.md#persistent-jobs--issue-3)に実施範囲と残件を記載しています。

```powershell
.\dist\agent-at.exe --persist --wake --at 05:00
.\dist\agent-at.exe --persist --wake --agent claude --at 05:00
.\dist\agent-at.exe --persist --at 23:30 --prompt-file .\request.txt
.\dist\agent-at.exe --persist --at 23:30 --resume SESSION_ID
.\dist\agent-at.exe --list
.\dist\agent-at.exe --remove JOB_ID
```

Windowsタスクスケジューラへ一回予約を保存して終了します。登録成功後は全ターミナルを
閉じて構いません。待機する独自プロセスは残しません。指定時刻にPCが起動・非スリープで、
**同じユーザーがサインイン済み**である必要があります。画面ロック中も対象ですが、
電源が入っているだけで未サインインなら対象外です。PC再起動後も予約は保持され、時刻までに
再サインインしていれば対象です。端末終了・PC再起動・ロック中のnative実証は残件です。
電源OFF／スリープ／サインアウトで見逃した要求の追い掛け実行、スリープ解除、繰返し、
自動再試行は行いません。resumeも `--at` 必須で、日時・秒・offsetを一度だけ固定します。
登録中に指定時刻を過ぎた場合は成功と断定せず、残った状態を表示します。

persistは常にheadlessです。`--headless` は重複指定できますが、`--headless=false`、
`--new-console=true`、`--close-on-exit=true` は入力エラーです。通常要求／resumeの設定・
承認方針は従来どおり、wakeは既存の軽量化・実行timeoutを保ち、model省略時はclean CLI
defaultです。`--list`／`--remove` は予約入力と排他で、CLIが未導入でも認証検査なしで
使用できます。Linux／macOSおよびWSLのLinuxバイナリでは未対応として拒否します。
従来の端末内タイマーの動作は変えません。

登録表示のジョブID、タスク名、確定日時、agent／model方針、保存先を確認してください。
exit 0は**登録成功**で、要求の実行成功ではありません。登録後のCtrl+Cでは取消されません。
`--list` は予約・開始記録・完了結果とOS登録の欠落／不整合を表示します。
`--remove JOB_ID` は未開始予約を取消し、実行済みなら要求・結果・ログを削除します。
実行中は拒否します。OS側の削除失敗時はデータと局所的な取消記録を残して起動を防ぎ、
報告されたOS側の問題を解決してから削除を再実施できます。

保存先はWindowsが返す当該ユーザーのLocalAppData配下 `agent-at\jobs\JOB_ID` です。
要求、`stdout.log`／`stderr.log`、`started.json`／`result.json` を `--remove` まで保持します。
保護DACLで当該ユーザーだけにアクセスを制限します。promptやログは機密を含み得るため、
無選別に公開しないでください。開始記録後に停止し結果が残らなければ「開始済み／結果不明」
とし、自動再送しません。モデル側のexactly-once受付は保証せず、CLI内部の通信retryとも別です。

prompt-fileは登録時の内容を固定します。agent-at自身、選択CLI、必要な作業先／add-dirは
元の絶対pathに保持してください。exeの自己コピーはしません。保存する環境はPATHと
HOME、USERPROFILE、CODEX_HOME、CLAUDE_CONFIG_DIR、ANTHROPIC_CONFIG_DIR、
XDG_CONFIG_HOME、APPDATAだけで、相対pathは登録時に絶対化します。npm `.cmd` は保存PATH
からNodeを解決できる必要があります。他の環境変数はOS側を使い、proxy／CA等もその条件に
従います。端末内だけの秘密環境変数の保存・移植は未対応です。資格情報は実行時に元の場所から
読み、wake認証の期限切れ・非対応方式はログインや別課金へのfallbackをせず失敗します。
Windowsパスワード保存・昇格・常駐サービスは不要ですが、タスク登録権限、CLI認証と通信は
必要です。非対応schemaは実行せず、記録を保持します。

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


## 指定時刻に最小要求を1回送る（未リリース）

```powershell
.\dist\agent-at.exe --wake --at 05:00
.\dist\agent-at.exe --wake --agent claude --at 05:00 --wake-text "ready"
```

wakeは専用の一時cwdで常にheadless実行し、JSON文字列で区切った短い要求をstdinへ1回だけ
渡します。返してほしい文字列は既定で `ok`。`--wake-text` は空白だけでないUTF-8の1行で、
引用符・日本語・shell特殊文字も扱えます。`--wake-timeout` は正のGo duration、既定 `2m`。
予約待機時間は含みません。タイマーを開いたままにしてください。PCのスリープ解除やOS予約では
ありません。時計変更・スリープ復帰で期限を過ぎた場合も既存タイマーで1回だけ起動し、実際の起動時刻を表示します。

**wakeは通常のユーザー／プロジェクト設定を読みません。** `--model` 省略時はclean CLI default
であり、普段のモデルと異なる場合があります。特定モデルの利用枠を狙うなら `--model MODEL` を
明示します。別モデルへの切替、再試行、枠のポーリング、自動再予約はしません。
最小要求でも使用量が発生します。成功表示は要求の完了であり、5時間・週間枠の開始・リセット・
起点移動を保証しません。送信後のtimeoutも消費ゼロの証拠ではありません。

対象の起動方針はCodex **0.154.0**、Claude Code **2.1.268**で確認しています。
必要なflagがない旧版では、通常設定へ戻さず失敗します。Codexはignore-user-config、read-only、
approval neverとし、ツール・hooks・memory・plugins／Apps・Web検索・スキルcatalog注入・同梱
スキルを停止します。Claudeはsafe mode、空setting sources／tools、1turnを指定します。
Claudeはnonessential trafficも停止し、セッション名の補助推論を省きます。
両者とも長い開発指示を短い固定指示へ置換し、同じモデルのlow effortを選びます。
lowに対応しないモデルは再送せず失敗します。通常タスク／resumeの設定・承認動作は従来どおりです。

既存サブスク認証とproxy／CA、必須管理policyを維持します。別のAPI課金経路で要求を送らないよう、
起動直前に既存の認証ファイルを読み取り専用で検査し、API／provider／通常モデルの環境変数は
子だけから除きます。CodexはChatGPTの `auth.json`、Claudeはgateway／federationのない
サブスクOAuthファイル認証が必要です。keychainだけの認証、不明な方式、相対の認証home、
host管理providerは未対応エラーにします。macOSのClaude wakeもkeychain方式を確認できないため
未対応です。資格情報のコピー・移動、ログインや恒久設定の変更をagent-atは行いません。
CLI自身の認証更新・認証やmetadata cacheの更新は残り得ます。

認証・必須policy／hooks・内部初期化は残ります。ディスク探索とcontext注入は別で、Codexは
catalog停止後もユーザースキルのrootを調べる場合があります。実測と観測限界は
[検証記録](docs/VERIFICATION.md)を参照してください。

wakeとresume、prompt、prompt-file、明示cd／add-dir、headless=false、new-console=true、
close-on-exit=trueは併用不可です。headlessとno-auto-approveは冗長指定として許可し、
通常の自動承認は追加しません。wakeなしのwake-text／wake-timeoutは入力エラーです。
終了コードは入力エラー2、起動・認証エラー1、timeout124、取消130、通常終了は子の終了コード。
起動後もCtrl+Cで取り消せます。このwakeのprocess group／Windows Jobの子孫だけを終了させ、
子の終了後に一時領域を削除します。親の強制終了やOS停止では一時領域が残り得ます。
Unixで意図的にprocess groupから離脱した子はgroupの対象外です。

source treeの同梱buildにはwakeが入ります。上記公開v0.2.0 ZIPには含まれません。
この変更ではreleaseを公開しません。

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
