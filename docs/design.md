# Design Document

## JetBrains風「Find in Files」TUI（Go + Bubble Tea + ripgrep）

* Version: 0.1
* Status: Draft
* Author:（記入）
* Date: 2026-01-18

---

## 1. Overview

本プロジェクトは、JetBrains IDE の **Find in Files** に近い体験を
**VS Code / Cursor 拡張に依存せず**、TUI（Terminal UI）として提供する。

ユーザーはターミナル上で以下を行える：

* ワークスペース全体を対象とした全文検索
* 入力に応じたインクリメンタル検索
* ↑↓キーで検索結果を選択
* 選択結果の周囲をプレビュー
* Enter で VS Code / Cursor を指定位置で開く

---

## 2. Goals / Non-Goals

### Goals

* JetBrains Find in Files に近い UX（入力→結果→プレビュー）
* 高速検索（ripgrep を利用）
* 単体実行可能な CLI / TUI
* VS Code / Cursor のどちらでも結果を開ける

### Non-Goals

* 独自検索インデックスの実装
* 正規表現の高度な UI 支援
* Windows PowerShell 特有の挙動最適化（初期は macOS / Linux 想定）

---

## 3. Technology Stack

| 領域     | 技術                             |
| ------ | ------------------------------ |
| 言語     | Go                             |
| TUI    | Bubble Tea                     |
| レイアウト  | lipgloss                       |
| 検索     | ripgrep (`rg`)                 |
| プレビュー  | Go 標準I/O（必要に応じて bat）           |
| エディタ起動 | `code --goto`, `cursor --goto` |

---

## 4. High-Level Architecture

```
┌─────────────────────────────┐
│         Bubble Tea UI       │
│                             │
│  Query / Mask Input         │
│        ↓                    │
│  Search Controller          │
│        ↓                    │
│  ripgrep (rg) process       │
│        ↓                    │
│  Result Parser              │
│        ↓                    │
│  Results Model ──────────┐  │
│        ↓                 │  │
│  Preview Loader           │  │
│        ↓                 │  │
│  Preview Model            │  │
│                             │
└─────────────┬───────────────┘
              │ Enter
              ↓
     VS Code / Cursor (--goto)
```

---

## 5. User Interaction Flow

1. 起動

   ```
   $ fif
   ```
2. クエリ入力（即時検索開始）
3. 結果一覧が更新される
4. ↑↓キーで結果選択
5. 選択変更に応じてプレビュー更新
6. Enter → エディタで該当位置を開く
7. Esc / Ctrl+C で終了

---

## 6. UI Layout（TUI）

```
┌ Query: <text>     Mask: <glob> ┐
│────────────────────────────────│
│ > file1.go:42  err != nil {    │
│   file2.go:18  foo := bar      │
│   ...                          │
│────────────────────────────────│
│  37 | func main() {            │
│  38 |   foo := 1               │
│  39 |   if err != nil {        │
│  40 |     panic(err)           │
└────────────────────────────────┘
```

* 上段：入力エリア（Query / Mask）
* 中段：検索結果リスト
* 下段：プレビュー（前後N行）

---

## 7. Data Model

### SearchResult

```go
type SearchResult struct {
  File   string // 相対パス
  Line   int    // 1-based
  Column int
  Text   string // マッチ行
}
```

### Preview

```go
type Preview struct {
  File      string
  StartLine int
  Lines     []string
  HitLine   int
}
```

---

## 8. Search Execution Design

### ripgrep 呼び出し

```sh
rg --vimgrep --no-heading --color=never <query> --glob <mask>
```

出力例：

```
path/to/file.go:42:13:err != nil {
```

### Go 側での処理

* `exec.CommandContext` で `rg` を起動
* 検索世代IDを持ち、古い検索結果は破棄
* 標準出力を逐次読み取り → UI更新

---

## 9. Incremental Search & Debounce

* クエリ入力変更ごとに検索要求
* 200–300ms のデバウンス
* Context cancel による検索中断

```go
ctx, cancel := context.WithCancel(context.Background())
```

---

## 10. Result Navigation

* Bubble Tea の `KeyUp` / `KeyDown`
* 選択 index を model に保持
* 選択変更 → Preview 再ロード

---

## 11. Preview Loading

* 選択された `SearchResult` に基づく
* 対象ファイルを開き、前後 N 行を抽出

```go
const previewBefore = 5
const previewAfter  = 10
```

* マッチ行はスタイルで強調表示

---

## 12. Open in Editor

### 対応エディタ

* VS Code
* Cursor

### コマンド

```sh
code --goto file:line:column
cursor --goto file:line:column
```

### エディタ選択戦略

1. `--editor` フラグ指定
2. 環境変数 `FIF_EDITOR`
3. 自動判定（`which cursor` → `which code`）

---

## 13. Key Bindings

| Key    | Action          |
| ------ | --------------- |
| ↑ / ↓  | 結果選択            |
| Enter  | エディタで開く         |
| Tab    | Query / Mask 切替 |
| Esc    | 終了              |
| Ctrl+C | 強制終了            |

---

## 14. Error Handling

* `rg` 未インストール → 起動時にエラー表示
* 検索結果ゼロ → Results に “No matches”
* ファイル読み込み失敗 → Preview に警告表示

---

## 15. Performance Considerations

* `rg` を毎回フル実行（インデックスなし）
* 世代管理 + cancel で無駄な処理を抑制
* Preview は選択行のみロード

---

## 16. Project Structure（提案）

```
fif/
  main.go
  tui/
    model.go
    update.go
    view.go
  search/
    rg.go
    parser.go
  preview/
    loader.go
  editor/
    open.go
  config/
    flags.go
```

---

## 17. Future Enhancements

* 正規表現 ON/OFF トグル
* 大量ヒット時のページング
* 結果グルーピング（ファイル単位）
* 設定ファイル対応（~/.config/fif/config.toml）
* LSP 連携（将来）

---

## 18. Alternatives Considered

* VS Code Extension

  * IDE依存・配布が重い
* fzf + rg

  * 実装は簡単だが UI 制御が限定的
* 独自インデクサ

  * 実装コストが高すぎる

---

## 19. Conclusion

Go + Bubble Tea + ripgrep による TUI 実装は、

* JetBrains Find in Files に近い体験
* 高速・軽量
* エディタ非依存

という点で非常に有効な設計である。

---

次のステップとしては
**① 最小で動く Bubble Tea Model**
**② rg 実行＋結果パース**
どちらからコードを書き始めるか決める。

どっちを先に書きますか？
