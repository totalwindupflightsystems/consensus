// Package web provides a minimal web admin UI for the Consensus runtime.
//
// The web UI serves embedded HTML pages (dashboard, sessions, memory browser).
// All data operations go through the existing REST API. The server also provides
// an API proxy for the frontend to avoid CORS issues.
//
// axiom:trace work_item=polish-phase spec=specs/016-cli-interface.md plan=phase-1/task-1/step-1 impl=internal/web/server.go
package web

import (
	"net/http"
	"strings"
)

// Server serves the web admin UI.
type Server struct {
	mux    *http.ServeMux
	apiURL string
}

// NewServer creates a new web UI server.
func NewServer(apiURL string) *Server {
	apiURL = strings.TrimRight(apiURL, "/")
	s := &Server{
		mux:    http.NewServeMux(),
		apiURL: apiURL,
	}

	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/dashboard", s.handleDashboard)
	s.mux.HandleFunc("/sessions", s.handleSessions)
	s.mux.HandleFunc("/memory", s.handleMemory)
	s.mux.HandleFunc("/health", s.handleHealthPage)
	s.mux.HandleFunc("/api/", s.handleAPIProxy)

	return s
}

// Handler returns the http.Handler for mounting under a parent server.
func (s *Server) Handler() http.Handler {
	return s.corsMiddleware(s.mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.html(w, pageShell(
		"Consensus — Admin UI",
		s.apiURL,
		"",
		`<p>Welcome to the Consensus admin console. Use the navigation to manage sessions, inspect memory, and monitor system health.</p>`),
	)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	c := `
<div class="stat-grid">
  <div class="card"><div class="stat" id="active-sessions">—</div><div class="stat-label">Active Sessions</div></div>
  <div class="card"><div class="stat" id="pending-tasks">—</div><div class="stat-label">Pending Tasks</div></div>
  <div class="card"><div class="stat" id="pending-approvals">—</div><div class="stat-label">Pending Approvals</div></div>
</div>
<div class="card"><h2>Recent Sessions</h2><table><thead><tr><th>ID</th><th>Agent</th><th>Status</th><th>Iter</th><th>Created</th><th></th></tr></thead><tbody id="sessions-table"><tr><td colspan="6" class="loading">Loading…</td></tr></tbody></table></div>
<div class="card"><h2>Health</h2><pre id="health-status" class="loading">Checking…</pre></div>
<script>(async function(){
try {
  var h=await apiFetch('/api/v1/health');
  document.getElementById('health-status').textContent=JSON.stringify(h,null,2);
  var m=await apiFetch('/api/v1/metrics');
  document.getElementById('active-sessions').textContent=m.active_sessions||0;
  document.getElementById('pending-tasks').textContent=m.pending_tasks||0;
  document.getElementById('pending-approvals').textContent=m.pending_approvals||0;
  var sess=await apiFetch('/api/v1/sessions');
  var tb=document.getElementById('sessions-table');
  if(!sess.length){tb.innerHTML='<tr><td colspan="6" class="empty-state">No sessions</td></tr>';return}
  tb.innerHTML=sess.slice(0,10).map(function(s){
    return'<tr><td><code>'+truncate(s.id,10)+'</code></td><td>'+(s.agent_name||'—')+'</td><td><span class="'+statusClass(s.status)+'">'+s.status+'</span></td><td>'+(s.iteration||0)+'</td><td>'+formatTime(s.created_at)+'</td><td class="actions"><button onclick="window.location=\\'/sessions?id=\\''+s.id+'\\'">View</button></td></tr>'
  }).join('')
}catch(err){
  document.getElementById('health-status').innerHTML='<div class="error-box">'+err.message+'</div>'
}})();
</script>`
	s.html(w, pageShell("Dashboard — Consensus", s.apiURL, "dashboard", c))
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	c := `
<div class="card">
  <div class="toolbar">
    <h2>Agent Sessions</h2>
    <div class="actions">
      <select id="status-filter" onchange="filterSessions()"><option value="">All Statuses</option><option value="idle">Idle</option><option value="thinking">Thinking</option><option value="planning">Planning</option><option value="tool_exec">Tool Exec</option><option value="paused">Paused</option><option value="completed">Completed</option><option value="failed">Failed</option></select>
      <button class="btn-primary" onclick="showCreateModal()">+ New</button>
    </div>
  </div>
  <table><thead><tr><th>ID</th><th>Agent</th><th>Status</th><th>Goal</th><th>Iter</th><th>Tokens</th><th>Created</th><th>Actions</th></tr></thead><tbody id="sessions-table"><tr><td colspan="8" class="loading">Loading…</td></tr></tbody></table>
</div>
<div id="create-form" style="display:none">
  <h3>Create Session</h3>
  <div class="form-group"><label>Agent Name</label><input id="ca" placeholder="researcher"/></div>
  <div class="form-group"><label>Goal</label><input id="cg" placeholder="What should the agent do?"/></div>
  <div class="form-group"><label>Model</label><input id="cm" placeholder="gpt-4o"/></div>
  <div class="actions"><button class="btn-primary" onclick="doCreate()">Create</button><button onclick="hideModal()">Cancel</button></div>
</div>
<script>
var allS=[];
async function load(){
  try {
    allS=await apiFetch('/api/v1/sessions');
    renderS(allS);
  }catch(e){document.getElementById('sessions-table').innerHTML='<tr><td colspan="8"><div class="error-box">'+e.message+'</div></td></tr>'}
}
function filterSessions(){
  var s=document.getElementById('status-filter').value;
  renderS(s?allS.filter(function(x){return x.status===s}):allS);
}
function renderS(sessions){
  var tb=document.getElementById('sessions-table');
  if(!sessions.length){tb.innerHTML='<tr><td colspan="8" class="empty-state">No sessions</td></tr>';return}
  tb.innerHTML=sessions.map(function(s){
    var btns='<button onclick="showDetail(\\''+s.id+'\\')">View</button>';
    if(s.status==='idle'||s.status==='thinking') btns+='<button class="btn-danger" onclick="doCancel(\\''+s.id+'\\')">Cancel</button>'
    if(s.status==='paused') btns+='<button onclick="doResume(\\''+s.id+'\\')">Resume</button>'
    return'<tr><td><code>'+truncate(s.id,10)+'</code></td><td>'+(s.agent_name||'—')+'</td><td><span class="'+statusClass(s.status)+'">'+s.status+'</span></td><td>'+truncate(s.goal,30)+'</td><td>'+(s.iteration||0)+'</td><td>'+((s.tokens_used_in||0)+(s.tokens_used_out||0))+'</td><td>'+formatTime(s.created_at)+'</td><td class="actions">'+btns+'</td></tr>'
  }).join('')
}
function showCreateModal(){showModal(document.getElementById('create-form').innerHTML)}
async function doCreate(){
  var a=document.getElementById('ca').value.trim();
  var g=document.getElementById('cg').value.trim();
  var m=document.getElementById('cm').value.trim();
  if(!g){alert('Goal required');return}
  try{await apiFetch('/api/v1/sessions',{method:'POST',body:JSON.stringify({agent_name:a||'agent',goal:g,model_id:m||undefined})});hideModal();load()}catch(e){alert(e.message)}
}
async function showDetail(id){
  try{
    var s=await apiFetch('/api/v1/sessions/'+id);
    var mem=await apiFetch('/api/v1/sessions/'+id+'/memory');
    var h='<h3>'+s.agent_name+' ('+truncate(id,12)+')</h3>';
    h+='<p><strong>Status:</strong> <span class="'+statusClass(s.status)+'">'+s.status+'</span> | <strong>Iter:</strong> '+s.iteration+'</p>';
    h+='<p><strong>Goal:</strong> '+(s.goal||'—')+'</p>';
    h+='<p><strong>Tokens:</strong> '+(s.tokens_used_in||0)+' in / '+(s.tokens_used_out||0)+' out</p>';
    h+='<h4 style="margin-top:12px">Recent Memory</h4><div class="memory-content">'+mem.slice(0,10).map(function(m){return'[#'+m.id+'] <'+m.type+'> '+truncate(m.content||m.rendered_text||'',200)}).join('\\n\\n')+'</div>';
    h+='<div class="actions" style="margin-top:12px"><button onclick="hideModal()">Close</button></div>';
    showModal(h)
  }catch(e){alert(e.message)}
}
async function doCancel(id){if(!confirm('Cancel this session?'))return;try{await apiFetch('/api/v1/sessions/'+id,{method:'PATCH',body:JSON.stringify({status:'cancelled'})});load()}catch(e){alert(e.message)}}
async function doResume(id){try{await apiFetch('/api/v1/sessions/'+id,{method:'PATCH',body:JSON.stringify({status:'idle'})});load()}catch(e){alert(e.message)}}
load();
</script>`
	s.html(w, pageShell("Sessions — Consensus", s.apiURL, "sessions", c))
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	c := `
<div class="card">
  <div class="toolbar"><h2>Memory Browser</h2><div class="actions"><input id="session-lookup" placeholder="Session ID to inspect…" style="width:300px"/><button class="btn-primary" onclick="lookup()">Inspect</button></div></div>
  <div id="memory-results"><div class="empty-state"><h3>Look up a session</h3><p>Enter a session ID above to browse its memory events.</p></div></div>
</div>
<script>
async function lookup(){
  var id=document.getElementById('session-lookup').value.trim();
  if(!id)return;
  var c=document.getElementById('memory-results');
  c.innerHTML='<div class="loading">Loading…</div>';
  try{
    var s=await apiFetch('/api/v1/sessions/'+id);
    var m=await apiFetch('/api/v1/sessions/'+id+'/memory');
    var iter=await apiFetch('/api/v1/sessions/'+id+'/iterations');
    c.innerHTML='<p><strong>Session:</strong> '+s.agent_name+' | <strong>Goal:</strong> '+(s.goal||'—')+' | <strong>Status:</strong> <span class="'+statusClass(s.status)+'">'+s.status+'</span></p>';
    if(m.length){
      c.innerHTML+='<h3 style="margin-top:12px">Memory Events ('+m.length+')</h3><table><thead><tr><th>ID</th><th>Type</th><th>Content</th><th>Mode</th><th>Iter</th><th>Created</th></tr></thead><tbody>'+m.map(function(e){return'<tr><td>'+e.id+'</td><td>'+e.type+'</td><td>'+truncate(String(e.content||e.rendered_text||''),100)+'</td><td>'+(e.display_mode||'full')+'</td><td>'+(e.iteration_created||0)+'</td><td>'+formatTime(e.created_at)+'</td></tr>'}).join('')+'</tbody></table>'
    }
    if(iter.length){
      c.innerHTML+='<h3 style="margin-top:12px">Iterations</h3><table><thead><tr><th>ID</th><th>Created</th></tr></thead><tbody>'+iter.slice(0,20).map(function(i){return'<tr><td>'+i.iteration_id+'</td><td>'+formatTime(i.created_at)+'</td></tr>'}).join('')+'</tbody></table>'
    }
  }catch(e){c.innerHTML='<div class="error-box">'+e.message+'</div>'}
}
document.getElementById('session-lookup').addEventListener('keydown',function(e){if(e.key==='Enter')lookup()});
</script>`
	s.html(w, pageShell("Memory — Consensus", s.apiURL, "memory", c))
}

func (s *Server) handleHealthPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"healthy":true,"version":"consensus-0.1.0","ui":"web-admin"}`))
}

func (s *Server) html(w http.ResponseWriter, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte("<!DOCTYPE html>\n" + page))
}

func pageShell(title, apiURL, active, content string) string {
	dashClass, sessClass, memClass := "", "", ""
	switch active {
	case "dashboard": dashClass = " class=\"active\""
	case "sessions": sessClass = " class=\"active\""
	case "memory": memClass = " class=\"active\""
	}
	return `<html lang="en">
<head>
<meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/>
<title>` + title + `</title>
<style>
*,*::before,*::after{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f1117;color:#e1e4e8;min-height:100vh}
.header{background:#161b22;border-bottom:1px solid #30363d;padding:12px 20px;display:flex;align-items:center;gap:16px;position:sticky;top:0;z-index:100}
.header h1{font-size:18px;font-weight:600;color:#f0f6fc}
.header nav{display:flex;gap:8px;margin-left:auto}
.header nav a{padding:6px 14px;border-radius:6px;font-size:13px;font-weight:500;color:#8b949e;text-decoration:none;transition:background .15s,color .15s}
.header nav a:hover,.header nav a.active{background:#21262d;color:#f0f6fc}
.main{padding:24px 20px;max-width:1200px;margin:0 auto}
.card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:20px;margin-bottom:20px}
.card h2{font-size:16px;font-weight:600;margin-bottom:16px;color:#f0f6fc}
table{width:100%;border-collapse:collapse}
th{text-align:left;padding:8px 12px;font-size:12px;font-weight:600;color:#8b949e;text-transform:uppercase;border-bottom:1px solid #21262d}
td{padding:8px 12px;font-size:13px;border-bottom:1px solid #21262d}
tr:hover{background:#1c2128}
.badge{display:inline-block;padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600;text-transform:uppercase}
.badge-idle{background:#1a3a2a;color:#3fb950}
.badge-thinking,.badge-booting{background:#1a2e3a;color:#58a6ff}
.badge-planning,.badge-waiting_sub{background:#2a1a3a;color:#bc8cff}
.badge-tool_exec,.badge-executing{background:#3a2a1a;color:#d29922}
.badge-paused{background:#3a1a2a;color:#f85149}
.badge-completed{background:#1a3a2a;color:#3fb950}
.badge-failed{background:#3a1a1a;color:#f85149}
button,.btn{padding:6px 14px;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;border:1px solid #30363d;background:#21262d;color:#c9d1d9;transition:background .15s}
button:hover,.btn:hover{background:#30363d}
.btn-primary{background:#238636;border-color:#238636;color:#fff}
.btn-primary:hover{background:#2ea043}
.btn-danger{background:#da3633;border-color:#da3633;color:#fff}
.btn-danger:hover{background:#f85149}
input,select{padding:6px 12px;border-radius:6px;border:1px solid #30363d;background:#0d1117;color:#c9d1d9;font-size:13px}
input:focus,select:focus{outline:none;border-color:#58a6ff}
.form-group{margin-bottom:14px}.form-group label{display:block;font-size:12px;font-weight:600;color:#8b949e;margin-bottom:4px}
.stat{font-size:28px;font-weight:700;color:#f0f6fc}
.stat-label{font-size:12px;color:#8b949e;text-transform:uppercase;margin-top:4px}
.actions{display:flex;gap:8px;flex-wrap:wrap}
.empty-state{text-align:center;padding:40px 20px;color:#8b949e}
.loading{text-align:center;padding:40px;color:#8b949e}
.error-box{background:#3a1a1a;border:1px solid #f85149;border-radius:6px;padding:12px;margin-bottom:16px;color:#f85149;font-size:13px}
.memory-content{white-space:pre-wrap;font-family:monospace;font-size:12px;background:#0d1117;padding:12px;border-radius:6px;border:1px solid #30363d;max-height:300px;overflow-y:auto}
.stat-grid{display:grid;grid-template-columns:1fr 1fr 1fr;gap:20px;margin-bottom:20px}
.toolbar{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px}
.toolbar h2{margin-bottom:0!important}
.modal-overlay{display:none;position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.6);z-index:1000;align-items:center;justify-content:center}
.modal-overlay.active{display:flex}
.modal{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:24px;max-width:500px;width:90%}
.modal h3{color:#f0f6fc;margin-bottom:12px}
.modal h4{color:#f0f6fc}
@media(max-width:768px){.stat-grid{grid-template-columns:1fr}.header nav{flex-wrap:wrap}}
</style></head>
<body>
<header class="header"><h1>⚡ Consensus</h1>
<nav>
<a href="/dashboard"` + dashClass + `>Dashboard</a>
<a href="/sessions"` + sessClass + `>Sessions</a>
<a href="/memory"` + memClass + `>Memory</a>
</nav>
</header>
<main class="main">` + content + `</main>
<div class="modal-overlay" id="modal"><div class="modal" id="modal-content"></div></div>
<script>
var API_URL="` + apiURL + `";
var API_KEY=sessionStorage.getItem('consensus_api_key')||'';
function ensureAuth(){if(!API_KEY){var k=prompt('Enter your Consensus API key:');if(k){sessionStorage.setItem('consensus_api_key',k);location.reload()}}}
async function apiFetch(path,opts){opts=opts||{};var url=API_URL+path;var h={'Content-Type':'application/json',opts:opts};for(var k in opts.headers)h[k]=opts.headers[k];if(API_KEY)h['Authorization']='Bearer '+API_KEY;var r=await fetch(url,{method:opts.method||'GET',headers:h,body:opts.body});if(!r.ok)throw new Error(await r.text()||'HTTP '+r.status);return r.json()}
function formatTime(t){if(!t)return'—';var d=new Date(t);if(isNaN(d.getTime()))return t;return d.toLocaleString()}
function truncate(s,n){if(!s)return'';s=String(s);return s.length>n?s.slice(0,n)+'…':s}
function statusClass(s){return'badge badge-'+(s||'unknown')}
function showModal(h){document.getElementById('modal-content').innerHTML=h;document.getElementById('modal').classList.add('active')}
function hideModal(){document.getElementById('modal').classList.remove('active')}
document.addEventListener('DOMContentLoaded',function(){document.getElementById('modal').addEventListener('click',function(e){if(e.target===document.getElementById('modal'))hideModal()});ensureAuth()});
</script>
</body></html>`
}

// ============================================================================
// API Proxy
// ============================================================================

func (s *Server) handleAPIProxy(w http.ResponseWriter, r *http.Request) {
	targetPath := strings.TrimPrefix(r.URL.Path, "/api")
	targetURL := s.apiURL + targetPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, `{"error":"proxy error"}`, http.StatusInternalServerError)
		return
	}

	if auth := r.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
