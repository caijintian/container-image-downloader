package main

import (
	"crypto/rand"
	"database/sql"
	_ "modernc.org/sqlite"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type GuestToken struct {
	Code      string    `json:"code"`
	Downloads int       `json:"downloads"`
	Used      int       `json:"used"`
	Images    []string  `json:"images"` // Track downloaded image+platform combos
	CreatedAt time.Time `json:"created_at"`
}

type Session struct {
	Role   string // "admin" or "guest"
	Code   string // the guest token code, if applicable
	Expiry time.Time
}


type Config struct {
	AdminUser string `json:"admin_user"`
	AdminPass string `json:"admin_pass"`
}

var (
	db *sql.DB

	appConfig = Config{AdminUser: "admin", AdminPass: "caijintian666"}
	configMu  sync.Mutex
	configFile = "config.json"
)


func loadConfig() {
	var user, pass string
	err := db.QueryRow(`SELECT value FROM config WHERE key='admin_user'`).Scan(&user)
	db.QueryRow(`SELECT value FROM config WHERE key='admin_pass'`).Scan(&pass)
	
	if err == sql.ErrNoRows || user == "" {
		appConfig.AdminUser = "admin"
		appConfig.AdminPass = "caijintian666"
		saveConfig()
	} else {
		appConfig.AdminUser = user
		appConfig.AdminPass = pass
	}
}

func saveConfig() {
	db.Exec(`INSERT OR REPLACE INTO config (key, value) VALUES ('admin_user', ?)`, appConfig.AdminUser)
	db.Exec(`INSERT OR REPLACE INTO config (key, value) VALUES ('admin_pass', ?)`, appConfig.AdminPass)
}


var (
	sessions   = make(map[string]Session)
	mu         sync.Mutex
	tokens     = make(map[string]*GuestToken)
	tokensMu   sync.Mutex
	tokensFile = "tokens.json"
)

func init() {
	initDB()
	loadConfig()
	loadTokens()
}


func initDB() {
	var err error
	db, err = sql.Open("sqlite", "fetcher.db")
	if err != nil {
		log.Fatal(err)
	}
	db.Exec(`CREATE TABLE IF NOT EXISTS config (key TEXT PRIMARY KEY, value TEXT)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS tokens (code TEXT PRIMARY KEY, downloads INTEGER, used INTEGER, images TEXT, created_at DATETIME)`)
}

func loadTokens() {
	rows, err := db.Query(`SELECT code, downloads, used, images, created_at FROM tokens`)
	if err != nil {
		return
	}
	defer rows.Close()

	tokensMu.Lock()
	defer tokensMu.Unlock()
	tokens = make(map[string]*GuestToken)
	for rows.Next() {
		t := &GuestToken{}
		var imagesStr string
		rows.Scan(&t.Code, &t.Downloads, &t.Used, &imagesStr, &t.CreatedAt)
		if imagesStr != "" {
			json.Unmarshal([]byte(imagesStr), &t.Images)
		}
		tokens[t.Code] = t
	}
}

func saveTokenToDB(t *GuestToken) {
	imagesJSON, _ := json.Marshal(t.Images)
	db.Exec(`INSERT OR REPLACE INTO tokens (code, downloads, used, images, created_at) VALUES (?, ?, ?, ?, ?)`,
		t.Code, t.Downloads, t.Used, string(imagesJSON), t.CreatedAt)
}

func deleteTokenFromDB(code string) {
	db.Exec(`DELETE FROM tokens WHERE code=?`, code)
}


func generateRandomCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	return strings.ToUpper(fmt.Sprintf("%x", b))
}

func generateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

const loginHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3E%3Cpath fill='%231890ff' fill-rule='evenodd' d='M15.528 2.973a.75.75 0 0 1 .472.696v8.662a.75.75 0 0 1-.472.696l-7.25 2.9a.75.75 0 0 1-.556 0l-7.25-2.9A.75.75 0 0 1 0 12.331V3.669a.75.75 0 0 1 .471-.696L7.443.184l.01-.003.268-.108a.75.75 0 0 1 .558 0l.269.108.01.003zM10.404 2 4.25 4.461 1.846 3.5 8 1.039zm.22 1.566L15.147 1.412 8.387 4.119zM8 5.663v9.227l6.5-2.6v-7.92zM7.5 14.89V5.663l-6.5-2.6v7.92z'/%3E%3C/svg%3E">
    <title>登录 - 容器镜像站点下载系统</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #f0f2f5; background-image: url('https://gw.alipayobjects.com/zos/rmsportal/TVYTbAXSpQhwvGooXzIq.svg'); background-repeat: no-repeat; background-position: center 110px; background-size: 100%; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
        .login-box { width: 368px; text-align: center; }
        .logo { font-size: 33px; font-weight: 600; color: rgba(0,0,0,.85); margin-bottom: 12px; display: flex; align-items: center; justify-content: center; gap: 10px;}
        .logo svg { width: 44px; height: 44px; }
        .subtitle { font-size: 14px; color: rgba(0,0,0,.45); margin-bottom: 40px; }
        .input-group { margin-bottom: 24px; text-align: left; }
        input[type="text"], input[type="password"] { width: 100%; padding: 10px 15px; border: 1px solid #d9d9d9; border-radius: 2px; font-size: 16px; box-sizing: border-box; transition: all .3s; }
        input[type="text"]:focus, input[type="password"]:focus { outline: none; border-color: #40a9ff; box-shadow: 0 0 0 2px rgba(24,144,255,.2); }
        .btn { width: 100%; background-color: #1890ff; color: #fff; border: none; padding: 10px 0; border-radius: 2px; font-size: 16px; cursor: pointer; transition: all .3s; box-shadow: 0 2px 0 rgba(0,0,0,.045); text-shadow: 0 -1px 0 rgba(0,0,0,.12); }
        .btn:hover { background-color: #40a9ff; }
        .error { color: #ff4d4f; font-size: 14px; margin-bottom: 20px; text-align: left; display: flex; align-items: center; padding: 8px 15px; background: #fff2f0; border: 1px solid #ffccc7; border-radius: 2px; }
        .tabs { display: flex; margin-bottom: 24px; border-bottom: 1px solid #d9d9d9; }
        .tab { flex: 1; padding-bottom: 8px; cursor: pointer; color: rgba(0,0,0,.45); transition: all 0.3s; }
        .tab.active { border-bottom: 2px solid #1890ff; color: #1890ff; }

        .toast { position: fixed; top: -50px; left: 50%; transform: translateX(-50%); background: #fff; padding: 10px 16px; border-radius: 4px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); display: flex; align-items: center; gap: 8px; z-index: 9999; opacity: 0; transition: all 0.3s cubic-bezier(0.645, 0.045, 0.355, 1); }
        .toast.show { opacity: 1; top: 20px; }
        .toast-success i { color: #52c41a; }
        
        .modal-mask { position: fixed; top:0; right:0; bottom:0; left:0; background: rgba(0,0,0,0.45); z-index: 1000; display: none; align-items: center; justify-content: center; }
        .modal-mask.show { display: flex; }
        .modal { background: #fff; width: 520px; border-radius: 4px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); display: flex; flex-direction: column; }
        .modal-header { padding: 16px 24px; border-bottom: 1px solid #f0f0f0; display: flex; justify-content: space-between; align-items: center; }
        .modal-title { margin:0; font-size: 16px; font-weight: 500; }
        .modal-close { cursor: pointer; color: rgba(0,0,0,.45); font-size: 16px; }
        .modal-close:hover { color: rgba(0,0,0,.75); }
        .modal-body { padding: 24px; }
        .modal-footer { padding: 10px 16px; border-top: 1px solid #f0f0f0; text-align: right; }
        .copy-area { display: flex; gap: 8px; margin-bottom: 16px; }
    </style>

</head>
<body>
    <div class="login-box">
        <div class="logo">
            <svg viewBox="0 0 24 24" fill="none" stroke="#1890ff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>
            容器镜像下载系统
        </div>
        
        {{if .Error}}
        <div class="error">
            <svg viewBox="0 0 24 24" fill="currentColor" style="width:16px; height:16px; margin-right:8px;"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z"/></svg>
            {{.Error}}
        </div>
        {{end}}

        <div class="tabs">
            <div id="tab-guest" class="tab active" onclick="switchTab('guest')">自助提货 (卡密)</div>
            <div id="tab-admin" class="tab" onclick="switchTab('admin')">管理后台</div>
        </div>

        <form id="form-guest" action="/login" method="POST">
            <div class="input-group">
                <input type="text" name="code" placeholder="请输入提货码 (例如 A8B9C2)" value="{{.Code}}" required autofocus>
            </div>
            <button type="submit" class="btn">提取镜像</button>
        </form>

        <form id="form-admin" action="/login" method="POST" style="display:none;">
            <div class="input-group">
                <input type="text" name="username" placeholder="账户" required>
            </div>
            <div class="input-group">
                <input type="password" name="password" placeholder="密码" required>
            </div>
            <button type="submit" class="btn">登 录</button>
        </form>
        
        <div style="margin-top: 45px; text-align: center;">
            <span style="font-family: 'Consolas', 'Courier New', monospace; color: #999; font-size: 13px; font-style: italic;">By </span>
            <span style="font-family: 'STXingkai', 'Xingkai SC', '华文行楷', 'Microsoft YaHei', cursive; font-size: 19px; font-weight: bold; color: #1890ff; letter-spacing: 2px; text-shadow: 1px 1px 2px rgba(24,144,255,0.2);">
                运维之魔
            </span>
        </div>
    </div>

    <div class="modal-mask" id="link-modal">
        <div class="modal">
            <div class="modal-header">
                <h4 class="modal-title">🎉 生成免密直链成功</h4>
                <i class="bi bi-x-lg modal-close" onclick="closeModal()"></i>
            </div>
            <div class="modal-body">
                <div class="alert alert-info" style="margin-bottom:16px;">
                    <i class="bi bi-info-circle-fill" style="margin-right:8px;"></i>
                    此链接不扣减访客提货次数，仅供管理员全网分发使用。
                </div>
                <div style="margin-bottom:8px; font-weight:500;">🌐 直链地址：</div>
                <div class="copy-area">
                    <input type="text" id="modal-link-input" class="ant-input" readonly>
                    <button class="ant-btn ant-btn-primary" onclick="copyModalInput('modal-link-input')">复制</button>
                </div>
                <div style="margin-bottom:8px; font-weight:500;">💻 一键 wget 下载命令：</div>
                <div class="copy-area" style="margin-bottom:0;">
                    <input type="text" id="modal-wget-input" class="ant-input" readonly>
                    <button class="ant-btn ant-btn-primary" onclick="copyModalInput('modal-wget-input')">复制</button>
                </div>
            </div>
            <div class="modal-footer">
                <button class="ant-btn" onclick="closeModal()">关 闭</button>
            </div>
        </div>
    </div>

    <script>

        function switchTab(tab) {
            document.getElementById('tab-admin').className = (tab === 'admin') ? 'tab active' : 'tab';
            document.getElementById('tab-guest').className = (tab === 'guest') ? 'tab active' : 'tab';
            document.getElementById('form-admin').style.display = (tab === 'admin') ? 'block' : 'none';
            document.getElementById('form-guest').style.display = (tab === 'guest') ? 'block' : 'none';
        }
        {{if .IsAdminMode}}
            switchTab('admin');
        {{end}}

        async function loadConfigData() {
            const res = await fetch('/api/admin/config');
            const data = await res.json();
            if(data.admin_user) {
                document.getElementById('cfg-user').value = data.admin_user;
            }
        }

        async function updateConfig(e) {
            e.preventDefault();
            const u = document.getElementById('cfg-user').value;
            const p = document.getElementById('cfg-pass').value;
            if(!confirm('修改凭证后您将被强制登出，是否继续？')) return;
            
            const formData = new URLSearchParams();
            formData.append('admin_user', u);
            formData.append('admin_pass', p);
            
            const res = await fetch('/api/admin/config', {
                method: 'POST',
                headers: {'Content-Type': 'application/x-www-form-urlencoded'},
                body: formData
            });
            if(res.ok) {
                alert('安全凭证修改成功，请使用新密码重新登录！');
                window.location.href = '/login';
            } else {
                alert('修改失败');
            }
        }
    </script>

</body>
</html>`

const dashboardHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%3E%3Cpath fill='%231890ff' fill-rule='evenodd' d='M15.528 2.973a.75.75 0 0 1 .472.696v8.662a.75.75 0 0 1-.472.696l-7.25 2.9a.75.75 0 0 1-.556 0l-7.25-2.9A.75.75 0 0 1 0 12.331V3.669a.75.75 0 0 1 .471-.696L7.443.184l.01-.003.268-.108a.75.75 0 0 1 .558 0l.269.108.01.003zM10.404 2 4.25 4.461 1.846 3.5 8 1.039zm.22 1.566L15.147 1.412 8.387 4.119zM8 5.663v9.227l6.5-2.6v-7.92zM7.5 14.89V5.663l-6.5-2.6v7.92z'/%3E%3C/svg%3E">
    <title>容器镜像站点下载系统</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap-icons@1.10.5/font/bootstrap-icons.css">
    <style>
        :root { --primary-color: #1890ff; --bg-color: #f0f2f5; --border-color: #f0f0f0; --text-color: #333; --sidebar-bg: #001529; --sidebar-text: rgba(255, 255, 255, 0.65); }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; margin: 0; padding: 0; background-color: var(--bg-color); color: var(--text-color); display: flex; height: 100vh; overflow: hidden; }
        
        /* Sidebar */
        .sidebar { width: 256px; background-color: var(--sidebar-bg); display: flex; flex-direction: column; transition: width 0.2s; }
        .sidebar .logo { height: 64px; display: flex; align-items: center; justify-content: center; font-size: 16px; font-weight: 600; color: #fff; background-color: #002140; gap: 10px;}
        .sidebar .logo svg { width: 28px; height: 28px; }
        .sidebar .menu { flex: 1; padding: 16px 0; }
        .sidebar .menu-item { padding: 12px 24px; color: var(--sidebar-text); cursor: pointer; display: flex; align-items: center; font-size: 14px; transition: 0.3s; }
        .sidebar .menu-item:hover { color: #fff; }
        .sidebar .menu-item.active { background-color: #1890ff; color: #fff; }
        .sidebar .menu-item i { margin-right: 10px; font-size: 16px; }
        
        /* Main Area */
        .main { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
        
        /* Header */
        .header { height: 64px; background: #fff; padding: 0 24px; display: flex; align-items: center; justify-content: space-between; box-shadow: 0 1px 4px rgba(0,21,41,.08); z-index: 10; }
        .header-title { font-size: 16px; font-weight: 500; }
        .header-right { display: flex; align-items: center; gap: 16px; cursor: pointer; }
        .header-right a { color: rgba(0,0,0,.65); text-decoration: none; }
        
        /* Content */
        .content { flex: 1; overflow-y: auto; padding: 24px; }
        .breadcrumb { margin-bottom: 16px; font-size: 14px; color: rgba(0,0,0,.45); }
        .breadcrumb span { color: rgba(0,0,0,.85); }
        
        /* Card */
        .card { background: #fff; border-radius: 2px; padding: 24px; box-shadow: 0 1px 2px -2px rgba(0,0,0,.16), 0 3px 6px 0 rgba(0,0,0,.12); }
        
        /* Toolbar */
        .toolbar { display: flex; gap: 16px; margin-bottom: 24px; flex-wrap: wrap; }
        .ant-input { padding: 4px 11px; border: 1px solid #d9d9d9; border-radius: 2px; font-size: 14px; line-height: 1.5715; transition: all 0.3s; flex: 1; min-width: 250px; }
        .ant-input:focus { border-color: #40a9ff; box-shadow: 0 0 0 2px rgba(24,144,255,.2); outline: 0; }
        .ant-select { padding: 4px 11px; border: 1px solid #d9d9d9; border-radius: 2px; font-size: 14px; background: #fff; cursor: pointer; }
        .ant-select:focus { border-color: #40a9ff; outline: 0; }
        
        .ant-btn { line-height: 1.5715; position: relative; display: inline-block; font-weight: 400; white-space: nowrap; text-align: center; border: 1px solid transparent; box-shadow: 0 2px 0 rgba(0,0,0,.015); cursor: pointer; transition: all .3s cubic-bezier(.645,.045,.355,1); height: 32px; padding: 4px 15px; font-size: 14px; border-radius: 2px; background: #fff; border-color: #d9d9d9; color: rgba(0,0,0,.85); }
        .ant-btn:hover { color: #40a9ff; border-color: #40a9ff; }
        .ant-btn-primary { color: #fff; border-color: var(--primary-color); background: var(--primary-color); text-shadow: 0 -1px 0 rgba(0,0,0,.12); box-shadow: 0 2px 0 rgba(0,0,0,.045); }
        .ant-btn-primary:hover { color: #fff; background: #40a9ff; border-color: #40a9ff; }
        .ant-btn-danger { color: #ff4d4f; border-color: #ff4d4f; }
        .ant-btn-danger:hover { color: #ff7875; border-color: #ff7875; }
        
        /* Table/List */
        .list-header { display: flex; padding: 16px 0; border-bottom: 1px solid var(--border-color); font-weight: 500; color: rgba(0,0,0,.85); }
        .list-item { display: flex; padding: 16px 0; border-bottom: 1px solid var(--border-color); transition: background 0.3s; align-items: center; }
        .list-item:hover { background: #fafafa; }
        .col-name { flex: 3; font-weight: 500; color: var(--primary-color); cursor: pointer; display: flex; align-items: center; gap: 8px;}
        .col-name:hover { text-decoration: underline; }
        .col-desc { flex: 5; color: rgba(0,0,0,.65); font-size: 14px; }
        .col-stats { flex: 2; display: flex; gap: 8px; }
        
        .tag { box-sizing: border-box; margin: 0; padding: 0 7px; font-size: 12px; line-height: 20px; white-space: nowrap; background: #fafafa; border: 1px solid #d9d9d9; border-radius: 2px; color: rgba(0,0,0,.65); }
        .tag-green { color: #52c41a; background: #f6ffed; border-color: #b7eb8f; }
        .tag-blue { color: #1890ff; background: #e6f7ff; border-color: #91d5ff; }
        
        #status { margin-bottom: 16px; font-size: 14px; }
        .alert { padding: 8px 15px; border-radius: 2px; margin-bottom: 16px; font-size: 14px; display: flex; align-items: center; }
        .alert-info { background-color: #e6f7ff; border: 1px solid #91d5ff; color: #1890ff; }
        .alert-error { background-color: #fff2f0; border: 1px solid #ffccc7; color: #ff4d4f; }
        .alert-success { background-color: #f6ffed; border: 1px solid #b7eb8f; color: #52c41a; }
        
        @keyframes spin { 100% { transform: rotate(360deg); } }

        .toast { position: fixed; top: -50px; left: 50%; transform: translateX(-50%); background: #fff; padding: 10px 16px; border-radius: 4px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); display: flex; align-items: center; gap: 8px; z-index: 9999; opacity: 0; transition: all 0.3s cubic-bezier(0.645, 0.045, 0.355, 1); }
        .toast.show { opacity: 1; top: 20px; }
        .toast-success i { color: #52c41a; }
        
        .modal-mask { position: fixed; top:0; right:0; bottom:0; left:0; background: rgba(0,0,0,0.45); z-index: 1000; display: none; align-items: center; justify-content: center; }
        .modal-mask.show { display: flex; }
        .modal { background: #fff; width: 520px; border-radius: 4px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); display: flex; flex-direction: column; }
        .modal-header { padding: 16px 24px; border-bottom: 1px solid #f0f0f0; display: flex; justify-content: space-between; align-items: center; }
        .modal-title { margin:0; font-size: 16px; font-weight: 500; }
        .modal-close { cursor: pointer; color: rgba(0,0,0,.45); font-size: 16px; }
        .modal-close:hover { color: rgba(0,0,0,.75); }
        .modal-body { padding: 24px; }
        .modal-footer { padding: 10px 16px; border-top: 1px solid #f0f0f0; text-align: right; }
        .copy-area { display: flex; gap: 8px; margin-bottom: 16px; }
    </style>

</head>
<body>
    <div class="sidebar">
        <div class="logo">
            <svg viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>
            容器镜像下载系统
        </div>
        <div class="menu">
            <div id="menu-search" class="menu-item active" onclick="switchView('search')"><i class="bi bi-box-seam"></i> 镜像仓库检索</div>
            {{if eq .Role "admin"}}
            <div id="menu-tokens" class="menu-item" onclick="switchView('tokens')"><i class="bi bi-ticket-perforated"></i> 提货卡密管理 (计费)</div>
            <div id="menu-config" class="menu-item" onclick="switchView('config')"><i class="bi bi-shield-lock"></i> 安全与账户设置</div>
            {{end}}
        </div>
    </div>
    
    <div class="main">
        <div class="header">
            <div class="header-title"><i class="bi bi-list" style="font-size:20px; cursor:pointer;"></i></div>
            <div class="header-right">
                {{if eq .Role "guest"}}
                <span class="tag tag-blue"><i class="bi bi-ticket-detailed"></i> 剩余下载次数: {{.Remaining}}</span>
                <span>访客 ({{.Code}})</span>
                {{else}}
                <span><i class="bi bi-person-circle" style="color:var(--primary-color);"></i> admin</span>
                {{end}}
                <a href="/logout"><i class="bi bi-box-arrow-right"></i></a>
            </div>
        </div>
        
        <div class="content">
            <div id="view-search" style="display:block;">

                      {{if eq .Role "admin"}}
                      <div style="display: flex; gap: 16px; margin-bottom: 24px;">
                          <div style="flex: 1; background: #fff; padding: 24px; border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.03); border: 1px solid #f0f0f0;">
                              <div style="color: #8c8c8c; font-size: 14px; margin-bottom: 12px;"><i class="bi bi-eye"></i> 平台总访问量</div>
                              <div style="font-size: 32px; font-weight: 600; color: #1890ff; line-height: 1;" id="stat-visits">-</div>
                          </div>
                          <div style="flex: 1; background: #fff; padding: 24px; border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.03); border: 1px solid #f0f0f0;">
                              <div style="color: #8c8c8c; font-size: 14px; margin-bottom: 12px;"><i class="bi bi-ticket-perforated"></i> 累计发行卡密</div>
                              <div style="font-size: 32px; font-weight: 600; color: #52c41a; line-height: 1;" id="stat-tokens">-</div>
                          </div>
                          <div style="flex: 1; background: #fff; padding: 24px; border-radius: 8px; box-shadow: 0 1px 2px rgba(0,0,0,0.03); border: 1px solid #f0f0f0;">
                              <div style="color: #8c8c8c; font-size: 14px; margin-bottom: 12px;"><i class="bi bi-cloud-arrow-down"></i> 累计成功拉取</div>
                              <div style="font-size: 32px; font-weight: 600; color: #722ed1; line-height: 1;" id="stat-used">-</div>
                          </div>
                      </div>
                      <script>
                          fetch('/api/admin/stats').then(r=>r.json()).then(d => {
                              if(!d.error) {
                                  document.getElementById('stat-visits').innerText = d.total_visits || 0;
                                  document.getElementById('stat-tokens').innerText = d.total_tokens || 0;
                                  document.getElementById('stat-used').innerText = d.total_used || 0;
                              }
                          });
                      </script>
                      {{end}}

                <div class="breadcrumb">
                    基础设施 / 容器镜像服务 / <span>全网镜像检索</span>
                </div>
                
                <div class="card">
                    <div class="toolbar">
                        <input type="text" id="query" class="ant-input" placeholder="输入镜像资产名称 (例如 nginx, mysql)" onkeypress="if(event.keyCode===13) searchImage()">
                        
                        <select id="arch" class="ant-select">
                            <option value="">架构: 默认探测</option>
                            <option value="linux/amd64">AMD64 (x86_64)</option>
                            <option value="linux/arm64">ARM64 (aarch64)</option>
                        </select>
                        
                        <button class="ant-btn ant-btn-primary" onclick="searchImage()"><i class="bi bi-search"></i> 执行检索</button>
                        <button class="ant-btn" onclick="directPull()"><i class="bi bi-cloud-download"></i> 强制同步下载</button>
                        {{if eq .Role "admin"}}
                        <button class="ant-btn" onclick="generateShareLink()"><i class="bi bi-share"></i> 分发免密直链</button>
                        {{end}}
                    </div>
                    
                    <div id="status"></div>
                    
                    <div id="results">
                        <div style="text-align: center; padding: 60px 0; color: rgba(0,0,0,.25);">
                            <i class="bi bi-inbox" style="font-size: 48px;"></i>
                            <p style="margin-top: 8px;">系统就绪，等待下发检索指令</p>
                        </div>
                    </div>
                </div>
            </div>

            {{if eq .Role "admin"}}
            <div id="view-tokens" style="display:none;">
                <div class="breadcrumb">
                    基础设施 / 业务计费 / <span>提货卡密管理</span>
                </div>
                <div class="card">
                    <div style="display:flex; justify-content: space-between; margin-bottom: 24px;">
                        <h3 style="margin:0; font-weight:400;">生成的提货码列表</h3>
                        <div style="display: flex; align-items: center; gap: 8px;">
                                <span style="color: #595959;">提取次数:</span>
                                <input type="number" id="token-limit" class="ant-input" style="width: 80px; padding: 4px 11px; height: 32px;" value="1" min="1">
                                <button class="ant-btn ant-btn-primary" onclick="createToken()" style="height: 32px; padding: 4px 15px;"><i class="bi bi-plus-circle"></i> 新建提货码</button>
                            </div>
                    </div>
                    <div id="token-list">加载中...</div>
                </div>
            </div>

            <div id="view-config" style="display:none;">
                <div class="breadcrumb">
                    基础设施 / 业务计费 / <span>安全与账户设置</span>
                </div>
                <div class="card" style="max-width: 500px;">
                    <h3 style="margin-top:0; font-weight:400; margin-bottom: 24px;">修改管理员凭证</h3>
                    <div class="alert alert-info"><i class="bi bi-info-circle-fill" style="margin-right:8px;"></i>修改密码后，您需要重新登录。</div>
                    <form id="config-form" onsubmit="updateConfig(event)">
                        <div style="margin-bottom:16px;">
                            <label style="display:block; margin-bottom:8px; color:rgba(0,0,0,.85);">登录账户名</label>
                            <input type="text" id="cfg-user" class="ant-input" required>
                        </div>
                        <div style="margin-bottom:24px;">
                            <label style="display:block; margin-bottom:8px; color:rgba(0,0,0,.85);">新密码</label>
                            <input type="password" id="cfg-pass" class="ant-input" required>
                        </div>
                        <button type="submit" class="ant-btn ant-btn-primary">保存修改</button>
                    </form>
                </div>
            </div>
            {{end}}
        </div>
    </div>


    <div class="modal-mask" id="link-modal">
        <div class="modal">
            <div class="modal-header">
                <h4 class="modal-title">🎉 生成免密直链成功</h4>
                <i class="bi bi-x-lg modal-close" onclick="closeModal()"></i>
            </div>
            <div class="modal-body">
                <div class="alert alert-info" style="margin-bottom:16px;">
                    <i class="bi bi-info-circle-fill" style="margin-right:8px;"></i>
                    此链接不扣减访客提货次数，仅供管理员全网分发使用。
                </div>
                <div style="margin-bottom:8px; font-weight:500;">🌐 直链地址：</div>
                <div class="copy-area">
                    <input type="text" id="modal-link-input" class="ant-input" readonly>
                    <button class="ant-btn ant-btn-primary" onclick="copyModalInput('modal-link-input')">复制</button>
                </div>
                <div style="margin-bottom:8px; font-weight:500;">💻 一键 wget 下载命令：</div>
                <div class="copy-area" style="margin-bottom:0;">
                    <input type="text" id="modal-wget-input" class="ant-input" readonly>
                    <button class="ant-btn ant-btn-primary" onclick="copyModalInput('modal-wget-input')">复制</button>
                </div>
            </div>
            <div class="modal-footer">
                <button class="ant-btn" onclick="closeModal()">关 闭</button>
            </div>
        </div>
    </div>

    <script>

        let currentRole = '{{.Role}}';
        

        function showToast(msg) {
            const toast = document.createElement('div');
            toast.className = 'toast toast-success';
            toast.innerHTML = '<i class="bi bi-check-circle-fill"></i><span>' + msg + '</span>';
            document.body.appendChild(toast);
            toast.offsetHeight;
            toast.classList.add('show');
            setTimeout(() => {
                toast.classList.remove('show');
                setTimeout(() => toast.remove(), 300);
            }, 3000);
        }

        function closeModal() {
            document.getElementById('link-modal').classList.remove('show');
        }

        function copyModalInput(id) {
            const el = document.getElementById(id);
            el.select();
            document.execCommand('copy');
            showToast('已复制到剪贴板！');
        }

        function switchView
(view) {
            document.getElementById('view-search').style.display = (view === 'search') ? 'block' : 'none';
            if(document.getElementById('view-tokens')) {
                document.getElementById('view-tokens').style.display = (view === 'tokens') ? 'block' : 'none';
                document.getElementById('view-config').style.display = (view === 'config') ? 'block' : 'none';
            }
            
            document.getElementById('menu-search').className = (view === 'search') ? 'menu-item active' : 'menu-item';
            if(document.getElementById('menu-tokens')) {
                document.getElementById('menu-tokens').className = (view === 'tokens') ? 'menu-item active' : 'menu-item';
                document.getElementById('menu-config').className = (view === 'config') ? 'menu-item active' : 'menu-item';
                
                if(view === 'tokens') loadTokens();
                if(view === 'config') loadConfigData();
            }
        }

        async function searchImage() {
            const query = document.getElementById('query').value.trim();
            if(!query) {
                showToast('请输入镜像名称');
                return;
            }
            const resultsDiv = document.getElementById('results');
            const statusDiv = document.getElementById('status');
            statusDiv.innerHTML = '';
            
            resultsDiv.innerHTML = '<div style="text-align: center; padding: 40px 0; color: #1890ff;"><i class="bi bi-arrow-repeat" style="font-size: 24px; animation: spin 1s linear infinite; display: inline-block;"></i><p style="margin-top:10px;">正在跨越星辰大海检索仓库...</p></div>';
            
            try {
                const res = await fetch('/api/search?q=' + encodeURIComponent(query));
                const data = await res.json();
                
                if(data.error) {
                    resultsDiv.innerHTML = '<div class="alert alert-error"><i class="bi bi-x-circle-fill" style="margin-right:8px;"></i>' + data.error + '</div>';
                } else if(!data || data.length === 0) {
                    resultsDiv.innerHTML = '<div style="text-align:center; padding: 40px; color: #8c8c8c;">未找到任何结果 (可能是私有镜像或拼写错误)</div>';
                } else {
                    let html = '<div class="list-header"><div style="flex:3;">资产标识</div><div style="flex:4;">概要描述</div><div style="flex:2;">审计标签</div><div style="flex:2; text-align:right;">操作</div></div>';
                    data.forEach(item => {
                        let offBadge = item.IsOfficial === '[OK]' ? '<span class="badge badge-official">Official</span>' : '';
                        let starBadge = item.StarCount && item.StarCount !== '0' ? '<span class="badge badge-star"><i class="bi bi-star-fill" style="color:#faad14;"></i> ' + item.StarCount + '</span>' : '';
                        
                        html += '<div class="list-item">' +
                            '<div style="flex:3; font-weight: 500; color: #1890ff; word-break: break-all;">' +
                                '<i class="bi bi-box" style="margin-right:8px;"></i>' + item.Name +
                            '</div>' +
                            '<div style="flex:4; color: #595959; font-size: 13px; padding-right:10px;">' +
                                (item.Description || '') +
                            '</div>' +
                            '<div style="flex:2; display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">' +
                                offBadge + ' ' + starBadge +
                            '</div>' +
                            '<div style="flex:2; text-align:right;">' +
                                '<button class="ant-btn ant-btn-primary" onclick="searchTagsForRepo(\'' + item.Name + '\')" style="padding: 4px 12px; height: 32px; font-size: 12px;"><i class="bi bi-tags"></i> 查版本</button>' +
                            '</div>' +
                        '</div>';
                    });
                    resultsDiv.innerHTML = html;
                }
            } catch (err) {
                resultsDiv.innerHTML = '<div class="alert alert-error"><i class="bi bi-x-circle-fill" style="margin-right:8px;"></i>检索请求失败，请检查网络</div>';
            }
        }
        
        async function searchTagsForRepo(repoName) {
            const resultsDiv = document.getElementById('results');
            resultsDiv.innerHTML = '<div style="text-align: center; padding: 40px 0; color: #1890ff;"><i class="bi bi-arrow-repeat" style="font-size: 24px; animation: spin 1s linear infinite; display: inline-block;"></i><p style="margin-top:10px;">正在光速拉取 <b>' + repoName + '</b> 的历史版本列表...</p></div>';
            
            try {
                const res = await fetch('/api/tags?image=' + encodeURIComponent(repoName));
                const data = await res.json();
                
                if(!data || data.length === 0 || data.error) {
                    resultsDiv.innerHTML = '<div style="text-align:center; padding: 40px; color: #8c8c8c;">未找到该镜像的任何版本标签</div>';
                    return;
                }
                
                let html = '<div class="list-header" style="background: #fafafa; border-bottom: 2px solid #e8e8e8;">' +
                        '<div style="flex:1;">' +
                            '<button class="ant-btn" onclick="searchImage()" style="height:32px; padding: 4px 12px; font-size: 13px;"><i class="bi bi-arrow-left"></i> 返回上一页</button>' +
                            '<span style="margin-left: 15px; color: #595959; font-weight:normal;">正在显示 <b>' + repoName + '</b> 的可用版本 (Tags)：</span>' +
                        '</div>' +
                    '</div>' +
                    '<div class="list-header"><div style="flex:3;">版本标签 (Tags)</div><div style="flex:4;">说明</div><div style="flex:2;"></div><div style="flex:2; text-align:right;">操作</div></div>';
                
                data.forEach(item => {
                    html += '<div class="list-item">' +
                        '<div style="flex:3; font-weight: 500; color: #1890ff; word-break: break-all;">' +
                            '<i class="bi bi-tag" style="margin-right:8px;"></i>' + item.Name +
                        '</div>' +
                        '<div style="flex:4; color: #595959; font-size: 13px;">' +
                            item.Description +
                        '</div>' +
                        '<div style="flex:2;"></div>' +
                        '<div style="flex:2; text-align:right;">' +
                            '<button class="ant-btn ant-btn-primary" onclick="useImage(\'' + item.Name + '\')" style="padding: 4px 12px; height: 32px; font-size: 13px;"><i class="bi bi-check-lg"></i> 选用此版本</button>' +
                        '</div>' +
                    '</div>';
                });
                resultsDiv.innerHTML = html;
            } catch (err) {
                resultsDiv.innerHTML = '<div class="alert alert-error"><i class="bi bi-x-circle-fill" style="margin-right:8px;"></i>拉取版本失败，请检查网络</div>';
            }
        }
        
        function useImage(imgName) {
            document.getElementById('query').value = imgName;
            document.getElementById('results').innerHTML = '<div class="alert alert-success" style="margin:20px 0;"><i class="bi bi-check-circle-fill" style="margin-right:8px;"></i> <b>' + imgName + '</b> 已自动填入顶栏，您可以直接点击右上角【分发免密直链】或【强制同步下载】。</div>';
            window.scrollTo(0, 0);
        }

        function directPull() {
            const query = document.getElementById('query').value.trim();
            if(query) startDownload(query);
        }

        async function startDownload(img) {
            const statusDiv = document.getElementById('status');
            const arch = document.getElementById('arch').value;
            let url = '/api/validate?image=' + encodeURIComponent(img);
            if(arch) url += '&platform=' + encodeURIComponent(arch);
            
            statusDiv.innerHTML = '<div class="alert alert-info"><i class="bi bi-info-circle-fill" style="margin-right:8px;"></i>正在校验云端清单层数据...</div>';
            
            try {
                const res = await fetch(url);
                if(res.status === 403) {
                    statusDiv.innerHTML = '<div class="alert alert-error"><i class="bi bi-x-circle-fill" style="margin-right:8px;"></i>您的下载次数已用尽！请联系管理员获取新提货码。</div>';
                    return;
                }
                const data = await res.json();
                
                if(data.error) {
                    statusDiv.innerHTML = '<div class="alert alert-error"><i class="bi bi-x-circle-fill" style="margin-right:8px;"></i>' + data.error + '</div>';
                } else {
                    if (data.is_retry) {
                        statusDiv.innerHTML = '<div class="alert alert-success"><i class="bi bi-check-circle-fill" style="margin-right:8px;"></i>校验成功，正在为您恢复之前的下载进度 (未扣除新次数)。</div>';
                    } else {
                        statusDiv.innerHTML = '<div class="alert alert-success"><i class="bi bi-check-circle-fill" style="margin-right:8px;"></i>校验成功，高速下行通道已打开。如果您是访客，您的提货次数已被扣除 1 次。</div>';
                    }
                    let cleanImg = img;
                    let dlUrl = '/image/' + encodeURIComponent(cleanImg) + '.tar';
                    if(arch) dlUrl += '?platform=' + encodeURIComponent(arch);
                    
                    // Create an iframe to trigger download silently without navigating away
                    let iframe = document.createElement('iframe');
                    iframe.style.display = 'none';
                    iframe.src = dlUrl;
                    document.body.appendChild(iframe);
                    
                    if(currentRole === 'guest') {
                        setTimeout(() => { location.reload(); }, 3000); // refresh to show updated remaining count
                    }
                }
            } catch(e) {
                statusDiv.innerHTML = '<div class="alert alert-error"><i class="bi bi-x-circle-fill" style="margin-right:8px;"></i>网络握手失败</div>';
            }
        }
        
        function generateShareLink() {
            const query = document.getElementById('query').value.trim();
            if(!query) {
                alert('请先输入镜像名称');
                return;
            }
            const arch = document.getElementById('arch').value;
            let cleanImg = query;
            let link = window.location.origin + '/image/' + cleanImg + '.tar';
            if(arch) link += '?platform=' + encodeURIComponent(arch);
            
            document.getElementById('modal-link-input').value = link;
            document.getElementById('modal-wget-input').value = 'wget ' + link;
            document.getElementById('link-modal').classList.add('show');
        }

        // Token Management JS
        async function loadTokens() {
            const res = await fetch('/api/admin/tokens');
            const data = await res.json();
            const listDiv = document.getElementById('token-list');
            if(!data || data.length === 0) {
                listDiv.innerHTML = '<p style="color:rgba(0,0,0,.45);">暂无提货码，请点击右上角创建</p>';
                return;
            }
            let html = '<div class="list-header"><div style="flex:2;">提货卡密</div><div style="flex:1;">包含次数</div><div style="flex:1;">已使用</div><div style="flex:3;">一键登录链接</div><div style="flex:1;">操作</div></div>';
            data.forEach(t => {
                let link = window.location.origin + '/login?code=' + t.code;
                let statusTag = t.used >= t.downloads ? '<span class="tag">已耗尽</span>' : '<span class="tag tag-green">生效中</span>';
                html += '<div class="list-item">' +
                    '<div style="flex:2; font-family: monospace; font-size: 16px; font-weight:bold; color:var(--primary-color);">' + t.code + ' ' + statusTag + '</div>' +
                    '<div style="flex:1;">' + t.downloads + ' 次</div>' +
                    '<div style="flex:1;">' + t.used + ' 次</div>' +
                    '<div style="flex:3;"><a href="'+link+'" target="_blank" style="color:#1890ff; font-size:13px; text-decoration:none;"><i class="bi bi-link-45deg"></i> 复制提货链接</a></div>' +
                    '<div style="flex:1;"><button class="ant-btn ant-btn-danger" style="height:26px; padding:0 10px; font-size:12px;" onclick="deleteToken(\''+t.code+'\')">作废</button></div>' +
                '</div>';
            });
            listDiv.innerHTML = html;
        }

        async function createToken() {
            let limit = 1;
            const limitInput = document.getElementById('token-limit');
            if(limitInput) {
                limit = parseInt(limitInput.value) || 1;
            }
            const fd = new URLSearchParams();
            fd.append('limit', limit);
            await fetch('/api/admin/tokens', {method: 'POST', body: fd});
            loadTokens();
            showToast('✅ 成功生成新提货码，可提取 ' + limit + ' 次镜像');
        }

        async function deleteToken(code) {
            if(!confirm('确定作废提货码 ' + code + ' 吗？')) return;
            await fetch('/api/admin/tokens?code=' + encodeURIComponent(code), {method: 'DELETE'});
            loadTokens();
        }

        async function loadConfigData() {
            const res = await fetch('/api/admin/config');
            const data = await res.json();
            if(data.admin_user) {
                document.getElementById('cfg-user').value = data.admin_user;
            }
        }

        async function updateConfig(e) {
            e.preventDefault();
            const u = document.getElementById('cfg-user').value;
            const p = document.getElementById('cfg-pass').value;
            if(!confirm('修改凭证后您将被强制登出，是否继续？')) return;
            
            const formData = new URLSearchParams();
            formData.append('admin_user', u);
            formData.append('admin_pass', p);
            
            const res = await fetch('/api/admin/config', {
                method: 'POST',
                headers: {'Content-Type': 'application/x-www-form-urlencoded'},
                body: formData
            });
            if(res.ok) {
                alert('安全凭证修改成功，请使用新密码重新登录！');
                window.location.href = '/login';
            } else {
                alert('修改失败');
            }
        }
    </script>

</body>
</html>`

func getSession(r *http.Request) (Session, bool) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return Session{}, false
	}
	mu.Lock()
	defer mu.Unlock()
	sess, exists := sessions[cookie.Value]
	if !exists || time.Now().After(sess.Expiry) {
		return Session{}, false
	}
	return sess, true
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := getSession(r)
		if !ok {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		code := r.URL.Query().Get("code")
		tmpl, _ := template.New("login").Parse(loginHTML)
		data := struct {
			Error       string
			Code        string
			IsAdminMode bool
		}{
			Code:        code,
			IsAdminMode: false,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, data)
		return
	}

	r.ParseForm()
	user := r.FormValue("username")
	pass := r.FormValue("password")
	code := r.FormValue("code")

	// 1. Admin Login
	if user != "" && pass != "" {
		configMu.Lock()
		valid := (user == appConfig.AdminUser && pass == appConfig.AdminPass)
		configMu.Unlock()
		if valid {
			token := generateSessionToken()
			mu.Lock()
			sessions[token] = Session{Role: "admin", Expiry: time.Now().Add(24 * time.Hour)}
			mu.Unlock()

			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    token,
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		
		tmpl, _ := template.New("login").Parse(loginHTML)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, struct {
			Error       string
			Code        string
			IsAdminMode bool
		}{Error: "管理员用户名或密码错误", IsAdminMode: true})
		return
	}

	// 2. Guest (Token) Login
	if code != "" {
		code = strings.ToUpper(strings.TrimSpace(code))
		tokensMu.Lock()
		_, exists := tokens[code]
		tokensMu.Unlock()

		if exists {
			token := generateSessionToken()
			mu.Lock()
			sessions[token] = Session{Role: "guest", Code: code, Expiry: time.Now().Add(24 * time.Hour)}
			mu.Unlock()

			http.SetCookie(w, &http.Cookie{
				Name:     "session_token",
				Value:    token,
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		tmpl, _ := template.New("login").Parse(loginHTML)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, struct {
			Error       string
			Code        string
			IsAdminMode bool
		}{Error: "提货码无效", Code: code})
		return
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		mu.Lock()
		delete(sessions, cookie.Value)
		mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   "",
		Expires: time.Unix(0, 0),
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	sess, _ := getSession(r)
	remaining := 0
	if sess.Role == "guest" {
		tokensMu.Lock()
		if t, ok := tokens[sess.Code]; ok {
			remaining = t.Downloads - t.Used
		}
		tokensMu.Unlock()
	}

	tmpl, _ := template.New("dash").Parse(dashboardHTML)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, struct {
		Role      string
		Code      string
		Remaining int
	}{
		Role:      sess.Role,
		Code:      sess.Code,
		Remaining: remaining,
	})
}

// Token management API

func configHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := getSession(r)
	if sess.Role != "admin" {
		http.Error(w, `{"error":"Forbidden"}`, http.StatusForbidden)
		return
	}

	if r.Method == "GET" {
		configMu.Lock()
		defer configMu.Unlock()
		json.NewEncoder(w).Encode(map[string]string{"admin_user": appConfig.AdminUser})
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		newUser := r.FormValue("admin_user")
		newPass := r.FormValue("admin_pass")
		if newUser == "" || newPass == "" {
			http.Error(w, `{"error":"Empty fields"}`, http.StatusBadRequest)
			return
		}
		
		configMu.Lock()
		appConfig.AdminUser = newUser
		appConfig.AdminPass = newPass
		saveConfig()
		configMu.Unlock()

		// Invalidate admin sessions
		mu.Lock()
		for token, s := range sessions {
			if s.Role == "admin" {
				delete(sessions, token)
			}
		}
		mu.Unlock()

		w.Write([]byte(`{"success":true}`))
		return
	}
}

func tokensHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := getSession(r)
	if sess.Role != "admin" {
		http.Error(w, `{"error":"Forbidden"}`, http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	tokensMu.Lock()
	defer tokensMu.Unlock()

	if r.Method == "GET" {
		// Return array of tokens
		var list []GuestToken
		for _, t := range tokens {
			list = append(list, *t)
		}
		json.NewEncoder(w).Encode(list)
		return
	}

	if r.Method == "POST" {
		limitStr := r.FormValue("limit")
		limit, _ := strconv.Atoi(limitStr)
		if limit <= 0 {
			limit = 1
		}
		code := generateRandomCode()
		t := &GuestToken{
			Code:      code,
			Downloads: limit,
			Used:      0,
			Images:    []string{},
			CreatedAt: time.Now(),
		}
		tokens[code] = t
		saveTokenToDB(t)
		
		json.NewEncoder(w).Encode(map[string]string{"code": code})
		return
	}

	if r.Method == "DELETE" {
		code := r.URL.Query().Get("code")
		delete(tokens, code)
		
		w.Write([]byte(`{"success":true}`))
		return
	}
}



func adminStatsHandler(w http.ResponseWriter, r *http.Request) {
	sess, _ := getSession(r)
	if sess.Role != "admin" {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var totalVisits int
	db.QueryRow(`SELECT value FROM config WHERE key='total_visits'`).Scan(&totalVisits)

	var totalTokens, totalUsed sql.NullInt64
	db.QueryRow(`SELECT count(*), sum(used) FROM tokens`).Scan(&totalTokens, &totalUsed)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_visits": totalVisits,
		"total_tokens": totalTokens.Int64,
		"total_used":   totalUsed.Int64,
	})
}

func tagsHandler(w http.ResponseWriter, r *http.Request) {
	image := r.URL.Query().Get("image")
	if image == "" || strings.ContainsAny(image, ";&|$`\\\\") {
		http.Error(w, `{"error":"Invalid image"}`, http.StatusBadRequest)
		return
	}

	cmd := exec.Command("sh", "-c", fmt.Sprintf("crane ls '%s'", image))
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"Tags fetch failed"}`))
		return
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	
	// Reverse to put newest/latest at top (often appended last)
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	var results []map[string]interface{}
	limit := 100
	if len(lines) < limit {
		limit = len(lines)
	}

	for i := 0; i < limit; i++ {
		if lines[i] != "" {
			results = append(results, map[string]interface{}{
				"Name":        fmt.Sprintf("%s:%s", image, lines[i]),
				"Description": "Historical / Specific Tag",
				"StarCount":   "-",
				"IsOfficial":  "",
				"IsAutomated": "",
			})
		}
	}
	
	if results == nil {
	    results = []map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, `{"error":"Invalid query"}`, http.StatusBadRequest)
		return
	}

	url := fmt.Sprintf("https://hub.docker.com/v2/search/repositories?query=%s&page_size=15", query)
	resp, err := http.Get(url)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"Docker API search failed"}`))
		return
	}
	defer resp.Body.Close()

	var hubResp struct {
		Results []struct {
			RepoName         string `json:"repo_name"`
			ShortDescription string `json:"short_description"`
			StarCount        int    `json:"star_count"`
			IsOfficial       bool   `json:"is_official"`
			IsAutomated      bool   `json:"is_automated"`
		} `json:"results"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&hubResp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"Docker API decode failed"}`))
		return
	}

	var results []map[string]interface{}
	for _, item := range hubResp.Results {
		isOfficial := ""
		if item.IsOfficial {
			isOfficial = "[OK]"
		}
		isAutomated := "false"
		if item.IsAutomated {
			isAutomated = "true"
		}
		results = append(results, map[string]interface{}{
			"Name":        item.RepoName,
			"Description": item.ShortDescription,
			"StarCount":   fmt.Sprintf("%d", item.StarCount),
			"IsOfficial":  isOfficial,
			"IsAutomated": isAutomated,
		})
	}
	
	if results == nil {
	    results = []map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}


func getImageAndPlatform(r *http.Request) (string, string) {
	image := r.URL.Query().Get("image")
	if image == "" && strings.HasPrefix(r.URL.Path, "/image/") {
		image = strings.TrimPrefix(r.URL.Path, "/image/")
		if strings.HasSuffix(image, ".tar.gz") {
			image = strings.TrimSuffix(image, ".tar.gz")
		} else if strings.HasSuffix(image, ".tar") {
			image = strings.TrimSuffix(image, ".tar")
		}
	}
	platform := r.URL.Query().Get("platform")
	return image, platform
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	// 校验前先检查限额
	sess, _ := getSession(r)
	isRetry := false
	if sess.Role == "guest" {
		tokensMu.Lock()
		t, ok := tokens[sess.Code]
		
		image, platform := getImageAndPlatform(r)
		cacheKey := image
		if platform != "" {
			cacheKey += "|" + platform
		}
		
		for _, img := range t.Images {
			if img == cacheKey {
				isRetry = true
				break
			}
		}
		
		isLegacyExhausted := len(t.Images) == 0 && t.Used >= t.Downloads
		if !ok || isLegacyExhausted || (!isRetry && t.Used >= t.Downloads) {
			tokensMu.Unlock()
			http.Error(w, `{"error":"下载次数已用尽"}`, http.StatusForbidden)
			return
		}
		tokensMu.Unlock()
	}

	image, platform := getImageAndPlatform(r)
	if image == "" || strings.ContainsAny(image, ";&|$`\\") {
		http.Error(w, `{"error":"Invalid image"}`, http.StatusBadRequest)
		return
	}

	platformArg := ""
	if platform != "" && !strings.ContainsAny(platform, ";&|$`\\") {
		platformArg = fmt.Sprintf("--platform '%s'", platform)
	}

	cmdStr := fmt.Sprintf("crane manifest %s '%s'", platformArg, image)
	cmd := exec.Command("sh", "-c", cmdStr)
	err := cmd.Run()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"镜像未找到或拒绝访问"}`))
		return
	}
	if isRetry {
		w.Write([]byte(`{"success":true, "is_retry": true}`))
	} else {
		w.Write([]byte(`{"success":true}`))
	}
}

func downloadStreamHandler(w http.ResponseWriter, r *http.Request) {
	// Deduct quota for guests BEFORE streaming
	sess, _ := getSession(r)
	if sess.Role == "guest" {
		tokensMu.Lock()
		t, ok := tokens[sess.Code]
		
		image, platform := getImageAndPlatform(r)
		cacheKey := image
		if platform != "" {
			cacheKey += "|" + platform
		}
		
		isRetry := false
		for _, img := range t.Images {
			if img == cacheKey {
				isRetry = true
				break
			}
		}
		
		isLegacyExhausted := len(t.Images) == 0 && t.Used >= t.Downloads
		if !ok || isLegacyExhausted || (!isRetry && t.Used >= t.Downloads) {
			tokensMu.Unlock()
			http.Error(w, "Forbidden: Quota Exceeded", http.StatusForbidden)
			return
		}
		
		if !isRetry {
			t.Images = append(t.Images, cacheKey)
			t.Used++
			
		}
		tokensMu.Unlock()
	}

	image, platform := getImageAndPlatform(r)
	if image == "" || strings.ContainsAny(image, ";&|$`\\") {
		http.Error(w, "Invalid image", http.StatusBadRequest)
		return
	}

	platformArg := ""
	if platform != "" && !strings.ContainsAny(platform, ";&|$`\\") {
		platformArg = fmt.Sprintf("--platform '%s'", platform)
	}

	filename := strings.ReplaceAll(image, "/", "_")
	filename = strings.ReplaceAll(filename, ":", "_")
	if platform != "" {
		arch := strings.ReplaceAll(platform, "/", "_")
		filename = filename + "_" + arch
	}
	filename += ".tar"

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Accel-Buffering", "no")

	// Command to pull on the fly (crane layers are already compressed, no need for pigz)
	cmdStr := fmt.Sprintf("crane pull %s '%s' /dev/stdout", platformArg, image)
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdout = w
	cmd.Run()
}

func main() {
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/", requireAuth(indexHandler))
	http.HandleFunc("/api/admin/tokens", requireAuth(tokensHandler))
	http.HandleFunc("/api/admin/config", requireAuth(configHandler))
	http.HandleFunc("/api/search", requireAuth(searchHandler))
	http.HandleFunc("/api/tags", requireAuth(tagsHandler))
	http.HandleFunc("/api/admin/stats", requireAuth(adminStatsHandler))
	http.HandleFunc("/api/validate", requireAuth(validateHandler))
	http.HandleFunc("/download_stream", downloadStreamHandler)
	http.HandleFunc("/image/", downloadStreamHandler)

	log.Println("=> Enterprise Engine (Skyline Billing) started on :8989")
	log.Fatal(http.ListenAndServe(":8989", nil))
}
