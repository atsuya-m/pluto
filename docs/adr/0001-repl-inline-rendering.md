# ADR 0001: REPL は AltScreen を使わずインライン描画する

## Status

Accepted (MVP)

## Context

`pluto repl` で Bubble Tea の AltScreen (fullscreen) を使うかどうかを、操作感を見て決める必要があった。

## Decision

MVP では AltScreen を使わない。

- command 実行結果（`desc` の出力、送信した request、response、error）は `tea.Println` で端末の scrollback に流す
- 画面下部にはその時点の interactive view（command line / RPC selector / request editor / preview / spinner / response actions）だけを描画する

## Consequences

- Evans のような shell 感が残り、過去の response を端末のスクロールやコピーでそのまま扱える
- 巨大な message の編集では editor 自体をスクロールさせる必要がある（editor は表示行数を端末の高さに合わせて制限する）
- fullscreen の方が良いと判断した場合も、`tea.WithAltScreen()` を足して response viewer に viewport を入れるだけで切り替えられる
