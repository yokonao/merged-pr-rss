# OSS Merged PR RSS Feed

OSSプロジェクトの直近マージされたPull RequestをRSS形式で配信するシステムです。

## 機能

- 複数のOSSリポジトリから最新のマージされたPRを取得（各リポジトリ最大100件）
- マージされた日時の降順でソート
- RSS形式で出力
- GitHub Pagesで公開
- GitHub Actionsで毎日自動更新

## 設定

`config.yaml`ファイルで監視対象のリポジトリとRSS設定を管理できます。

```yaml
repositories:
  - owner: facebook
    name: react
    description: "A declarative, efficient, and flexible JavaScript library for building user interfaces."
  - owner: microsoft
    name: vscode
    description: "Visual Studio Code"
  # ... 他のリポジトリ
```

## 使用方法

### ローカル実行

```bash
# 依存関係のインストール
go mod tidy

# RSS生成の実行
go run main.go
```

### GitHub Pagesでの公開

1. GitHubリポジトリでPages機能を有効化
2. Source を "GitHub Actions" に設定
3. GitHub Actionsが自動的に毎日実行され、RSSフィードを更新

### RSSフィードURL

GitHub Pagesが有効になっている場合、以下のURLでRSSフィードにアクセスできます：

```
https://[username].github.io/[repository-name]/feed.xml
```

## GitHub Token

GitHub APIのレート制限を避けるため、`GITHUB_TOKEN`環境変数を設定することを推奨します：

```bash
export GITHUB_TOKEN=your_github_token_here
go run main.go
```

GitHub Actionsでは自動的に`GITHUB_TOKEN`が利用可能です。

## ファイル構成

```
├── main.go              # メインアプリケーション
├── config.yaml          # 設定ファイル
├── docs/
│   ├── feed.xml         # 生成されたRSSフィード
│   └── index.html       # HTMLインデックスページ
└── .github/workflows/
    └── update-rss.yml   # GitHub Actionsワークフロー
```

## カスタマイズ

- 監視対象リポジトリの追加/削除は`config.yaml`で設定
- RSS設定（タイトル、説明など）も`config.yaml`で変更可能
- 更新頻度は`.github/workflows/update-rss.yml`のcron設定で変更可能

## ライセンス

MIT LicenseConvert recently merged GitHub PRs into an RSS feed
