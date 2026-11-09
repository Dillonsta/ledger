# ledger

跑在 EdgePi E87N（ARM64）上的轻量自托管记账本：**Go 单二进制 + SQLite + 内嵌 Web UI**，Docker 镜像 ~20MB，常驻内存 ~20MB。

## 功能

- 登录认证（bcrypt + HMAC token，30 天有效）
- 记一笔：支出/收入、分类、账户、日期、备注；编辑、删除
- 明细：按月查看、按日分组、当月收支结余
- 图表：支出分类横条图、逐日柱状图、近 6 个月收支趋势折线
- 预算：按支出分类设月度预算，进度条（80% 变橙、超支变红）
- 设置：改密码、自定义分类/账户（有流水的禁止删除）
- 多人预留：所有表都按 `user_id` 隔离，后续加注册接口即可

## 快速部署（E87N / 任意 arm64 设备）

```sh
docker compose up -d --build
```

然后浏览器访问 `http://<设备IP>:8300`。

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

- `ledger.exe`（windows/amd64，本地测试用）、`ledger-arm64`（linux/arm64，E87N 用）：纯静态二进制，无 CGO
- 前端通过 `go:embed` 打进二进制，运行时无外部文件依赖

## 路线图（按需）

- [ ] 用户注册接口（表结构已就绪）
- [ ] CSV 导入导出
- [ ] PWA（添加到主屏）
- [ ] 多账户余额视图
