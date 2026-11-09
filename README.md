# ledger

跑在ARM64上的轻量自托管记账本：**Go 单二进制 + SQLite + 内嵌 Web UI**，Docker 镜像 ~10MB，常驻内存 ~20MB。

## 功能

- 登录认证（bcrypt + HMAC token，30 天有效）
- 记一笔：支出/收入、分类、账户、日期、备注；编辑、删除
- 明细：按月查看、按日分组、当月收支结余
- 图表：支出分类横条图、逐日柱状图、近 6 个月收支趋势折线
- 预算：按支出分类设月度预算，进度条（80% 变橙、超支变红）
- 设置：改密码、自定义分类/账户（有流水的禁止删除）
- 多人预留：所有表都按 `user_id` 隔离，后续加注册接口即可

## 快速部署（任意 arm64 设备）

```sh
docker compose up -d --build
```

然后浏览器访问 `http://<设备IP>:8300`。

### OpenWrt 系路由器的注意事项

1. **用 host 网络模式**。OpenWrt 固件的防火墙（DOCKER-MAN 链）会拦掉 LAN 到 bridge 网络容器的新连接，且 dockerd 重启后 DOCKER 链的端口规则可能不重建。改成：
   ```yaml
   services:
     ledger:
       image: ledger:1.0
       network_mode: host   # 直接监听宿主机 8300，绕开全部 NAT/转发链
       ...
   ```
2. **设备无外网时**（拉不动 Docker Hub / 克隆不了 GitHub），本机构建镜像再传过去：
   ```sh
   # 本机（Windows/macOS/Linux 均可）
   docker buildx build --platform linux/arm64 -t ledger:1.0 --load .
   docker save ledger:1.0 | gzip > ledger-arm64.tar.gz
   scp -O ledger-arm64.tar.gz root@<设备IP>:/mnt/nvme0n1p2/ledger/

   # 设备上
   docker load -i ledger-arm64.tar.gz
   docker compose up -d
   ```
3. scratch 镜像以 UID 65534（nobody）运行，数据目录需要先 `chown -R 65534:65534 <数据目录>`。

- 首次启动自动创建用户（环境变量 `LEDGER_ADMIN_USER` / `LEDGER_ADMIN_PASS`，默认 `admin` / `admin123`），并预置默认分类（餐饮/交通/购物…）和账户（现金/微信/支付宝/银行卡）
- **部署前改掉 `docker-compose.yml` 里的默认密码**
- 数据全在 `./data/`（`ledger.db` + `token.key`），备份 = 复制这个目录

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `LEDGER_DATA` | `/data` | 数据目录（SQLite + token 密钥） |
| `LEDGER_ADDR` | `:8300` | 监听地址 |
| `LEDGER_ADMIN_USER` | `admin` | 首次启动创建的用户名 |
| `LEDGER_ADMIN_PASS` | `admin123` | 首次启动创建的密码 |
| `TZ` | `Asia/Shanghai` | 时区 |

## 本地开发与交叉编译

不需要本机装 Go，用 Docker 里的工具链：

```sh
# 生成/更新依赖（写 go.sum）
docker run --rm -v "$PWD:/src" -w /src golang:1.26-alpine go mod tidy

# vet + 双平台构建（Windows 本地测试 + arm64 目标机）
docker run --rm -v "$PWD:/src" -w /src golang:1.26-alpine sh -c \
  "go vet ./... && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ledger.exe . && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o ledger-arm64 ."

# 本地跑（换端口换数据目录）
LEDGER_DATA=./testdata LEDGER_ADDR=127.0.0.1:18300 ./ledger.exe
```

## 冒烟测试

```sh
LEDGER_DATA=./testdata LEDGER_ADDR=127.0.0.1:18300 ./ledger.exe &   # 后台起服务
sh ./smoke.sh    # 25 项断言：登录/CRUD/预算/报表/静态页/权限
```

## 构建产物

- `ledger.exe`（windows/amd64，本地测试用）、`ledger-arm64`（linux/arm64，目标设备用）：纯静态二进制，无 CGO
- 前端通过 `go:embed` 打进二进制，运行时无外部文件依赖

## CSV 导入

设置页「CSV 导入」卡片：选择文件或粘贴文本 → 预览 → 导入。也支持 API 供 agent 自动化：

```sh
TOKEN=$(curl -s -X POST http://<设备IP>:8300/api/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"你的密码"}' | grep -o '"token":"[^"]*' | cut -d'"' -f4)

curl -X POST http://<设备IP>:8300/api/import \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"rows":[
    {"date":"2026-11-09","type":"支出","amount":"25.60","category":"餐饮","account":"微信","note":"午餐"},
    {"date":"2026-11-09","type":"收入","amount":"5000","category":"工资","account":"银行卡","note":"工资"}
  ]}'
```

规则：
- 列：`日期,类型,金额,分类,账户,备注`（前端有表头自动识别；API 直接收结构化 rows）
- 日期支持 `2026-11-09` / `2026/11/9` / `2026.11.09`；类型 `支出/收入`；金额单位元（`¥`、千分位逗号均可）
- 分类/账户不存在会自动创建；备注选填
- **幂等**：日期+类型+金额+分类+账户+备注完全相同的行视为重复，自动跳过 → agent 重试不会重复记账
- **全有或全无**：任何一行非法，整批回滚，返回 `{line, error}` 行级错误列表
- 单次上限 1000 行

## 路线图（按需）

- [ ] 用户注册接口（表结构已就绪）
- [ ] CSV 导出
- [ ] PWA（添加到主屏）
- [ ] 多账户余额视图
