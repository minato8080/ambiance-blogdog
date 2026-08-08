# 推定コスト

クローラーを週1回稼働（[design-infrastructure.md](./design-infrastructure.md) 参照）に変更した前提での、月額コスト概算。

> **注意**: 以下は 2026-08-08 時点の実測値・公開料金表をもとにした概算。各サービスの料金は変更されるため、正確な金額は各コンソールの請求情報（GCP Billing、Neon Dashboard、OpenAI Usage）で確認すること。

---

## 前提データ（実測）

- 本番DB: `blogs` 7,482件（ready 7,457 / pending 18 / error 7）、`articles` 22,509件（`/stats` より取得）
- Cloud Run Jobs リソース: 全フェーズ共通で 1 vCPU / 512Mi メモリ
- 実行時間（過去の実行ログより）
  | フェーズ | 典型実行時間 |
  | -------- | ------------ |
  | discovery | 約60〜70秒 |
  | indexer | 約50〜80秒 |
  | historical | 約30〜60秒 |
  | recent | 約20〜40秒 |
  | syncer | 約8〜41分（対象ブログ数・変更検知数により変動、最も重い） |

---

## 内訳

### Cloud Run Jobs（クローラー本体）

週1回 × 5フェーズの実行を月換算（約4.345週/月）すると、vCPU使用量は概算で:

- 週合計: 65 + 65 + 40 + 30 + 1,200（syncerを20分と仮定）秒 ≈ 1,400秒
- 月合計: 約 6,100 vCPU秒（≈ 1.7 vCPU時間）、メモリは約 3,000 GiB秒

Cloud Run の無料枠（月 180,000 vCPU秒 / 360,000 GiB秒、プロジェクト全体で共有）に対して数%程度の使用量にとどまるため、**実質 $0/月**。

> 参考: 以前の6時間・1時間・30分間隔の頻度でも、1回あたりの実行時間がごく短いため無料枠内に収まっていた可能性が高い。頻度を下げたことによるGCP課金上のメリットは小さく、主な効果は OpenAI API 呼び出し回数と Neon への書き込み量を抑えられる点。

### Cloud Scheduler

3ジョブ構成（gather / indexer / syncer）。無料枠（3ジョブ/月）以内に収まるため追加課金なし。

- **$0/月**
- 以前は discovery / indexer / syncer / historical / recent の5ジョブで $0.20/月 発生していたが、gather への統合で無料枠内に収まるようになった。

### OpenAI Embeddings API（`text-embedding-3-small`, $0.02 / 1M tokens）

- indexer は1回の実行で最大 `INDEX_BATCH_SIZE`(50)ブログ × `MAX_ARTICLES_PER_BLOG`(5)記事 = 最大250記事を新規Embedding化
- 現在の pending バックログは18ブログ（≈最大90記事）
- syncer は変更があった記事のみ再Embedding化（通常は少数）
- 1記事あたり `EMBED_MAX_CHARS`(1000文字) ≈ 400〜500トークンと仮定

週200記事のEmbedding化を仮定すると:

- 200記事 × 450トークン × 4.345週 ≈ 39万トークン/月
- 39万 / 1,000,000 × $0.02 ≈ **$0.01/月未満**

新規ブログの増加ペースが数倍になっても $0.05/月 程度で、**ほぼ無視できるコスト**。

### Cloud Run（API サーバー: `blogdog-api`）

`/similar` `/random` `/blogs` `/stats` `/keywords` を提供する常時稼働ではないサービス（min instance 未設定 = リクエストが無ければ課金なし）。

- 実際のリクエスト数は本ドキュメント作成時点で未計測（Cloud Monitoring / GCP Billing レポートで要確認）
- 無料枠（月200万リクエスト、180,000 vCPU秒、360,000 GiB秒）の範囲内であれば **$0/月**
- 個人利用・デモ規模のアクセス量であれば無料枠内に収まる可能性が高い

### Neon（PostgreSQL + pgvector）

- ストレージ概算: `embedding vector(1536)` は1レコードあたり約6KB。22,509記事 → 埋め込みデータのみで約138MB、インデックス（IVFFlat）や他カラムを含めると **概算300〜350MB**
- Neon Free Plan のストレージ上限（プランにより変動、要確認）に近づきつつあるため、記事数の増加ペースを監視推奨
- コンピュート時間（起動中の接続時間）による課金は本ドキュメントでは未計測。Neon Dashboard の Usage タブで確認すること

### Firebase Hosting

- `public/` 配下の静的ファイル（`admin.html`, `viewer.html`, `config.js`, `style.css`)のみ
- Spark（無料）プランの範囲（10GB保存 / 360MB転送/日）で十分収まる想定 → **$0/月**

---

## 合計目安

| 項目 | 月額目安 |
| ---- | -------- |
| Cloud Run Jobs（クローラー） | $0（無料枠内） |
| Cloud Scheduler | $0（無料枠3ジョブ以内） |
| OpenAI Embeddings API | $0.01未満 |
| Cloud Run（APIサーバー） | $0（無料枠内、要トラフィック確認） |
| Neon（PostgreSQL） | 未確認（ストレージ300〜350MB、無料枠に近い可能性） |
| Firebase Hosting | $0 |
| **合計** | **概ね $0.2〜1/月**（Neon・APIトラフィックが無料枠を超えない前提） |

クローラーを週1回にしたことで、GCPの計算コスト自体はもともと無料枠内だったため大きくは変わらないが、**Neonへの書き込み量とOpenAI APIの呼び出し頻度が下がる**ことで、データ増加ペース・API課金の両方を抑えられる。またdiscovery/historical/recentをgatherに統合したことでCloud Schedulerが無料枠（3ジョブ）に収まり、$0.20/月の削減になった。
