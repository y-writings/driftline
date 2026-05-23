# driftline

`driftline` は、あるディレクトリの管理対象ファイルを別のリポジトリへ同期し、同期状態をロックファイルで追跡するCLIツールです。

## インストール

現時点ではソースからビルドして使います。

```sh
go build ./src/cmd/driftline
```

Dockerイメージとしてビルドする場合は次を実行します。

```sh
docker build -t driftline .
```

## 設定

同期元ディレクトリに `driftline.yaml` を置きます。

```yaml
version: 1
gitignore:
  - .driftline.lock
files:
  - id: example
    source: templates/example.txt
    target: example.txt
  - id: local-config
    source: templates/config.local
    target: config.local
    if_not_exists: true
```

主な項目は次のとおりです。

- `version`: 現在は `1` のみ対応
- `gitignore`: `update` 時に同期先の `.gitignore` へ追記する項目
- `files`: 同期するファイル一覧
- `id`: ファイルを識別する一意なID
- `source`: 同期元ディレクトリからの相対パス
- `target`: 同期先ディレクトリからの相対パス
- `if_not_exists`: `true` の場合、同期先にファイルがあると上書きしない

`update` を実行すると、同期先に `.driftline.lock` が作成または更新されます。

## 使い方

基本形は次のとおりです。

```sh
driftline <command> [options]
```

よく使う例です。

```sh
driftline check --source-dir ../templates --target-dir .
driftline diff --source-dir ../templates --target-dir .
driftline update --source-dir ../templates --target-dir .
driftline prune --source-dir ../templates --target-dir .
```

コマンドは次のとおりです。

- `check`: 同期先が同期元と一致しているか確認する
- `diff`: 追加または更新されるファイルの差分を表示する
- `update`: 追加または更新が必要なファイルをコピーし、ロックファイルを更新する
- `prune`: マニフェストから削除済みで、同期先にローカル変更がないファイルを削除する

主なオプションは次のとおりです。

- `--source-dir`: 同期元ディレクトリ。既定値は `.`
- `--target-dir`: 同期先ディレクトリ。既定値は `.`
- `--manifest`: マニフェストの相対パス。既定値は `driftline.yaml`
- `--lock`: ロックファイルの相対パス。既定値は `.driftline.lock`
- `--repository`: ロックファイルに書き込むリポジトリ名
- `--ref`: ロックファイルに書き込む参照名

パスは各ディレクトリからの相対パスで指定します。絶対パスや `..` でルート外へ出るパスは使えません。
