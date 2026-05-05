# TempoTTY

純 Go 寫的終端機節奏遊戲。讀取音檔自動抽出鼓點/bass 產生譜面，在 TTY 上玩。

## Build

```bash
go build -o bin/tempo-tty ./cmd/tempo-tty   # 整合 TUI（推薦）
go build -o bin/analyze   ./cmd/analyze     # 分析 CLI
go build -o bin/play      ./cmd/play        # 直接玩 chart.json 的 CLI
```

## 玩

```bash
./bin/tempo-tty                      # 打開檔案選擇器
./bin/tempo-tty path/to/song.mp3     # 直接玩這首
./bin/tempo-tty https://youtu.be/…   # 從 YouTube 下載並玩
```

打開後流程：

```
[檔案選擇器] → [自動分析（首次）] → [遊戲] → [結算] → 回檔案選擇器
```

第一次選某首歌會自動分析並把譜面快取在音檔旁邊（`song.mp3` → `song.chart.json`）。
之後選同一首歌會秒進。要重新分析就在選擇器上把游標停在歌上按 `r`。

### YouTube 來源

> `yt-dlp` 與 `ffmpeg` 是**執行期**相依，不是編譯期。Release binary 不裝也能跑、本機音檔也能玩，只有要用 URL 時才需要這兩個工具。沒裝就用 URL 會吃到明確錯誤訊息提示 `brew install …`。

先安裝 `yt-dlp` 與 `ffmpeg`：
```bash
# macOS
brew install yt-dlp ffmpeg

# Linux
sudo apt install yt-dlp ffmpeg

# Windows
winget install yt-dlp.yt-dlp Gyan.FFmpeg
```

兩種用法：

```bash
# 1) 命令列直通
./bin/tempo-tty 'https://youtu.be/dQw4w9WgXcQ'
./bin/tempo-tty 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# 2) 進入檔案選擇器後按 u 貼網址
./bin/tempo-tty
```

下載後音檔快取在 `~/.cache/tempo-tty/<videoID>.mp3`，譜面快取在 `~/.cache/tempo-tty/<videoID>.chart.json`，下次同一支秒進。
yt-dlp 支援的網站不只 YouTube（Bandcamp、SoundCloud 等也行，未全測）。

設定（包含 offset、預設目錄、按鍵等）寫入 `~/.config/tempo-tty/config.json`。

## 操作

### 檔案選擇器
| 鍵 | 功能 |
|----|------|
| `↑↓` / `j k` | 上下移動 |
| `Enter` | 進入資料夾 / 選歌 |
| `u` | 貼上 URL（YouTube 等） |
| `r` | 強制重新分析該首歌 |
| `h` | 跳到家目錄 |
| `PgUp/PgDn` | 翻頁 |
| `Esc` / `q` | 結束 |

### 遊戲中
| 鍵 | 功能 |
|----|------|
| `D F J K`（預設） | 對應 4 條 lane |
| `[` / `]` | 即時調整 offset ∓5ms |
| `\` | offset 歸零 |
| `q` / `Esc` | 結束本曲 |

頭顯右上會即時顯示 `off:+0.080`。回到結算後 offset 會自動寫進 config，下次自動帶入。

## 判定

| 等級 | 容差 | 分數 |
|------|------|------|
| PERFECT | ±50ms | 300 |
| GREAT | ±100ms | 200 |
| GOOD | ±150ms | 100 |
| MISS | 超過 | 0 |

每 25 連擊分數倍率 +100%。

## CLI 用法（不走 TUI）

```bash
# 分析
./bin/analyze song.mp3 -o chart.json [-lanes 4] [-min-gap 0.08]

# 玩
./bin/play chart.json [-keys dfjk] [-speed 3.5] [-offset 0]
```

## 結構

```
cmd/
  tempo-tty/   整合 TUI 入口（picker → analyze → game）
  analyze/     離線分析 CLI
  play/        播放單一 chart.json 的 CLI
internal/
  audio/       音檔解碼 + sample-counter audio clock
  onset/       spectral flux onset detection、lane 分配、BPM 估算
  analyze/     onset → chart 的整合層
  chart/       譜面 JSON 格式
  game/        遊戲主迴圈（接受外部 tcell screen）
  ui/          檔案選擇器、訊息畫面、結算畫面
  config/      使用者偏好持久化
```

## 演算法

**Onset detection**：
1. 解碼為 mono samples
2. STFT (frame 2048 / hop 512、Hann window)
3. spectral flux：相鄰 frame 幅度譜的正向差總和
4. peak picking：local maxima + 區域均值自適應閾值 + 最小間隔
5. 取每個 onset 點的 spectral centroid，依分位數切到 N 條 lane

**音訊同步**：`internal/audio.Counter` 包裝 streamer，原子計數已送進 speaker 的樣本數。
位置 = 累計樣本 / sample rate，buffer 50ms，玩家用 `[`/`]` 即時校正。

## Tradeoff / 之後可改

- **無 hold note**（長按音）。
- **無 calibration mode**：目前靠遊玩中 `[`/`]` 校正，之後可做開頭節拍器自動量測平均誤差。
- **快取譜面寫在音檔旁**：唯讀目錄會失敗（會落地到 fallback 嗎？目前沒有，會略過快取繼續玩）。
- **沒做難度分級**，可由 `-min-gap` 與 `-lanes` 參數推導。
- **BPM 估算簡陋**（IOI 中位數折回 60–180），複雜節奏會偏掉，目前只用於 HUD。
