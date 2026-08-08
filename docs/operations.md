# 運用手順

## Cloud Scheduler ジョブ管理

クローラーの定期実行は Cloud Scheduler で管理している。以下のコマンドで一時停止・再開が可能。

### 前提

`gcloud` が Python 3.10 のクラッシュ問題を抱えている場合は、先頭に環境変数を設定する。

```powershell
$env:CLOUDSDK_PYTHON = "C:\Users\fujin\AppData\Local\Programs\Python\Python310\python.exe"
```

### ジョブ一覧確認

```powershell
gcloud scheduler jobs list --location=asia-northeast1 --project=ambiance-blogdog
```

### 全ジョブ停止

```powershell
foreach ($job in @("discovery", "indexer", "syncer", "historical", "recent")) {
  gcloud scheduler jobs pause "blogdog-crawler-$job" --location=asia-northeast1 --project=ambiance-blogdog
}
```

### 全ジョブ再開

```powershell
foreach ($job in @("discovery", "indexer", "syncer", "historical", "recent")) {
  gcloud scheduler jobs resume "blogdog-crawler-$job" --location=asia-northeast1 --project=ambiance-blogdog
}
```

### 個別ジョブの停止・再開

```powershell
# 停止
gcloud scheduler jobs pause blogdog-crawler-{フェーズ名} --location=asia-northeast1 --project=ambiance-blogdog

# 再開
gcloud scheduler jobs resume blogdog-crawler-{フェーズ名} --location=asia-northeast1 --project=ambiance-blogdog
```

フェーズ名: `discovery` / `indexer` / `syncer` / `historical` / `recent`

---

## ジョブの手動実行

スケジュールを待たずに即時実行したい場合は Cloud Run Jobs を直接トリガーする。

```powershell
gcloud run jobs execute blogdog-crawler --update-env-vars CRAWLER_PHASE={フェーズ名} --region=asia-northeast1 --project=ambiance-blogdog
```

---

## コンソールリンク

- Cloud Scheduler: https://console.cloud.google.com/cloudscheduler?project=ambiance-blogdog
- Cloud Run Jobs: https://console.cloud.google.com/run/jobs?project=ambiance-blogdog
