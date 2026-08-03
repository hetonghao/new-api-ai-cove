# AI Cove Responses WebSocket zstd v1

`ai-cove-zstd.v1` 是 AI Cove Turbo 与 AI Cove New API 之间的私有 WebSocket 子协议。Codex 客户端不可见该协议。Codex 直连 New API 或经 Turbo 走标准透明回退时仍可协商 `permessage-deflate`；Turbo 私有模式的本地回环腿可以不接受该扩展，由公网腿 zstd 提供压缩收益。

## 协商与回退

- Turbo 在公网腿握手中提出 `Sec-WebSocket-Protocol: ai-cove-zstd.v1`。
- 私有协议握手不提出 `permessage-deflate`；New API 接受私有子协议时也不协商该扩展，避免 zstd envelope 被二次压缩。
- New API 接受后，双方所有应用消息都使用下述二进制 envelope；Ping、Pong、Close 仍是标准 WebSocket 控制帧。
- New API 未接受该子协议时，Turbo 仅可在尚未发送应用消息前重连标准透明 WebSocket。
- 已发送应用消息后发生协议错误时，双方关闭连接，不静默重放；是否回退 HTTP 由 Codex 自身决定。

## Envelope

每个应用消息独立编码为一个 WebSocket Binary message。头部固定 10 字节，整数使用网络字节序：

| Offset | 长度 | 字段 | 约束 |
|---|---:|---|---|
| 0 | 4 | Magic | ASCII `AICZ` |
| 4 | 1 | Version | `0x01` |
| 5 | 1 | Flags | bit 0：zstd；bit 1：原始 Binary；其余位必须为 0 |
| 6 | 4 | Original length | 解压后的原始消息长度，uint32 big-endian |
| 10 | N | Payload | zstd 数据或原始消息 |

- bit 1 未设置时，原始消息类型为 Text，内容必须是合法 UTF-8。
- zstd 使用 level 3。只有压缩结果严格小于原始消息时才设置 bit 0；否则发送原始 payload。
- 未压缩 payload 长度必须等于 Original length；压缩 payload 长度必须小于 Original length；解压结果长度必须精确等于 Original length。
- Original length 与 wire payload 均不得超过 128 MiB。

确定性未压缩文本向量：原始 Text `ok` 对应十六进制 `4149435a0100000000026f6b`。

压缩互操作向量：将文本 `{"type":"response.create","input":[]}` 重复 256 次后，合法 envelope 十六进制为 `4149435a01010000250028b52ffd6000247d010054027b2274797065223a22726573706f6e73652e637265617465222c22696e707574223a5b5d7d0154160531c52628`。接收方必须能解码该向量；发送方不要求生成逐字节相同的 zstd frame。

## 关闭码

- `1002`：非 Binary 应用帧、magic/version/flags、未压缩长度或压缩收益规则错误。
- `1007`：zstd 数据损坏、解压后长度不符、Text 解码后不是合法 UTF-8。
- `1009`：声明长度或 wire payload 超过 128 MiB。

该协议不占用 RSV 位，也不声明或冒充 `permessage-zstd`。
