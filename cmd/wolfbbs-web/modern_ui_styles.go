package main

const modernUIStyleTag = `<style id="wolfbbs-modern-ui">
:root{
  --bg:#f5f8fc;
  --bg-alt:#eef4fb;
  --surface:#ffffff;
  --surface-2:#f8fbff;
  --surface-3:#edf4ff;
  --text:#122035;
  --muted:#556579;
  --line:#d3deec;
  --line-strong:#b8c8de;
  --accent:#0a5cc6;
  --accent-strong:#07408a;
  --accent-soft:#d8e7ff;
  --teal:#0f9274;
  --ok:#1f7a49;
  --warn:#9a6700;
  --danger:#b12b3b;
  --shadow-sm:0 6px 14px rgba(12,27,50,.08);
  --shadow:0 14px 34px rgba(12,27,50,.12);
  --shadow-lg:0 22px 58px rgba(10,23,44,.18);
  --radius:16px;
  --radius-sm:12px;
  --radius-pill:999px;
}
*{box-sizing:border-box}
html,body{height:100%}
body{
  margin:0 auto;
  width:min(1260px,calc(100% - 2.6rem));
  padding:26px 0 80px;
  color:var(--text);
  font:15px/1.5 "Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  background:
    radial-gradient(900px 340px at 6% -14%, rgba(10,92,198,.18) 0%, transparent 64%),
    radial-gradient(740px 260px at 94% -10%, rgba(15,146,116,.14) 0%, transparent 62%),
    radial-gradient(660px 230px at 50% -22%, rgba(255,183,82,.16) 0%, transparent 68%),
    linear-gradient(180deg,var(--bg),var(--bg-alt));
  -webkit-font-smoothing:antialiased;
  text-rendering:optimizeLegibility;
}
body::before{
  content:"";
  position:fixed;
  inset:0;
  pointer-events:none;
  z-index:-1;
  opacity:.32;
  background:
    linear-gradient(rgba(255,255,255,.55), rgba(255,255,255,.55)),
    repeating-linear-gradient(90deg, rgba(11,58,126,.03) 0px, rgba(11,58,126,.03) 1px, transparent 1px, transparent 28px),
    repeating-linear-gradient(0deg, rgba(11,58,126,.025) 0px, rgba(11,58,126,.025) 1px, transparent 1px, transparent 28px);
}
h1,h2,h3{
  margin:0 0 11px;
  line-height:1.18;
  letter-spacing:.01em;
  color:#0f2949;
  font-family:"Sora","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
}
h1{
  font-size:1.9rem;
  font-weight:780;
  letter-spacing:.012em;
}
h2{
  font-size:1.24rem;
  font-weight:720;
}
h3{
  font-size:1.03rem;
  font-weight:700;
}
p,ul,ol,table,form,section,article,pre{margin:0 0 14px}
a{
  color:var(--accent);
  text-decoration:none;
  text-underline-offset:2px;
}
a:hover{color:var(--accent-strong);text-decoration:underline}
body > h1:first-of-type{
  margin-bottom:10px;
  letter-spacing:.016em;
}
body > p:first-of-type{
  color:var(--muted);
}
p.wolfbbs-nav-row,body > p:has(> a){
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  padding:11px 12px;
  background:linear-gradient(180deg,rgba(255,255,255,.95),rgba(246,251,255,.94));
  border:1px solid var(--line);
  border-radius:var(--radius-sm);
  box-shadow:var(--shadow-sm);
  backdrop-filter:blur(5px);
}
p.wolfbbs-nav-row a,body > p:has(> a) a{
  display:inline-flex;
  align-items:center;
  justify-content:center;
  min-height:32px;
  padding:6px 13px;
  border-radius:var(--radius-pill);
  border:1px solid #c7d9f1;
  background:linear-gradient(180deg,#ffffff,#eff6ff);
  color:#154681;
  font-weight:600;
  font-size:.9rem;
}
p.wolfbbs-nav-row a:hover,body > p:has(> a) a:hover{
  transform:translateY(-1px);
  background:linear-gradient(180deg,#eff6ff,#e1ecff);
  border-color:#98b8e3;
  text-decoration:none;
}
table{
  width:100%;
  border-collapse:separate;
  border-spacing:0;
  background:var(--surface);
  border:1px solid var(--line);
  border-radius:var(--radius-sm);
  overflow:hidden;
  box-shadow:var(--shadow-sm);
}
th,td{
  padding:10px 12px;
  text-align:left;
  border-bottom:1px solid #e6eef8;
  vertical-align:top;
}
th{
  background:linear-gradient(180deg,#eff5fd,#e6effa);
  color:#1f3a5b;
  font-weight:700;
  font-size:.87rem;
  text-transform:uppercase;
  letter-spacing:.04em;
}
tr:nth-child(even) td{background:#f9fcff}
tr:last-child td{border-bottom:0}
form{
  background:linear-gradient(180deg,var(--surface),#fafdff);
  border:1px solid var(--line);
  border-radius:var(--radius-sm);
  padding:15px;
  box-shadow:var(--shadow-sm);
}
table form{
  margin:0;
  padding:0;
  background:none;
  border:0;
  border-radius:0;
  box-shadow:none;
}
label{
  display:inline-flex;
  flex-direction:column;
  gap:6px;
  margin:0 11px 10px 0;
  font-weight:600;
  color:#2a415e;
}
input[type=text],input[type=password],input[type=email],input[type=number],input[type=url],input[type=search],select,textarea{
  width:min(100%,520px);
  min-height:40px;
  border-radius:11px;
  border:1px solid #bacedf;
  background:#fff;
  color:var(--text);
  padding:9px 11px;
  font:inherit;
  transition:border-color .18s ease, box-shadow .18s ease, background-color .18s ease;
}
input::placeholder,textarea::placeholder{
  color:#7590ae;
}
textarea{min-height:110px;resize:vertical}
input:focus,select:focus,textarea:focus{
  outline:0;
  border-color:#2f77d3;
  background:#ffffff;
  box-shadow:0 0 0 3px rgba(47,119,211,.17);
}
button,input[type=submit],input[type=button]{
  border:1px solid transparent;
  border-radius:11px;
  min-height:38px;
  padding:9px 15px;
  cursor:pointer;
  font:700 .91rem/1 "Sora","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  color:#fff;
  background:linear-gradient(140deg,#0a66d6,#07469a 56%,#0f9274);
  box-shadow:0 8px 18px rgba(9,71,154,.24);
  transition:transform .16s ease, box-shadow .16s ease, filter .16s ease;
}
button:hover,input[type=submit]:hover,input[type=button]:hover{
  transform:translateY(-1px);
  box-shadow:0 12px 20px rgba(9,71,154,.29);
  filter:saturate(1.05);
}
button:active,input[type=submit]:active,input[type=button]:active{
  transform:translateY(0);
}
code,pre{
  font-family:"SFMono-Regular","Menlo","Consolas",monospace;
}
pre{
  padding:11px 13px;
  border:1px solid #c7d7ec;
  border-radius:11px;
  background:linear-gradient(180deg,#fafdff,#f2f7ff);
  overflow:auto;
}
blockquote{
  margin:0 0 12px;
  padding:10px 13px;
  border-left:4px solid #95b9e8;
  border-radius:0 10px 10px 0;
  background:linear-gradient(180deg,#f7fbff,#eff5ff);
  color:#224265;
}
hr{
  border:0;
  border-top:1px solid var(--line);
  margin:16px 0;
}
.wolfbbs-muted{color:var(--muted)}
.wolfbbs-kpi-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(140px,1fr));
  gap:12px;
  margin:16px 0 20px;
}
.wolfbbs-kpi-card,.wolfbbs-card,.wolfbbs-action-card{
  background:var(--surface);
  border:1px solid var(--line);
  border-radius:var(--radius);
  box-shadow:var(--shadow);
}
.wolfbbs-kpi-card{
  position:relative;
  overflow:hidden;
  padding:17px 18px;
  display:flex;
  flex-direction:column;
  gap:4px;
  background:
    radial-gradient(circle at 108% -28%, rgba(10,92,198,.17), transparent 56%),
    linear-gradient(180deg,#ffffff,#f5faff);
}
.wolfbbs-kpi-card strong{
  font-size:1.72rem;
  line-height:1;
  letter-spacing:.01em;
}
.wolfbbs-kpi-card span{
  color:var(--muted);
  font-weight:600;
}
.wolfbbs-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(260px,1fr));
  gap:15px;
  margin:15px 0 20px;
}
.wolfbbs-card-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(220px,1fr));
  gap:12px;
}
.wolfbbs-card{
  padding:17px 18px;
  background:
    radial-gradient(circle at 106% -30%, rgba(13,103,214,.07), transparent 56%),
    linear-gradient(180deg,#ffffff,#f8fbff);
}
.wolfbbs-card > :last-child{
  margin-bottom:0;
}
.wolfbbs-primer{
  position:relative;
  overflow:hidden;
  padding:20px 21px;
  margin:15px 0 20px;
  background:
    radial-gradient(circle at top right, rgba(10,92,198,.2), transparent 38%),
    radial-gradient(circle at 82% 112%, rgba(15,146,116,.15), transparent 42%),
    linear-gradient(180deg,#ffffff,#f1f7ff);
  border:1px solid #bfd2eb;
  box-shadow:var(--shadow-lg);
}
.wolfbbs-primer::after{
  content:"";
  position:absolute;
  inset:auto -30px -60px auto;
  width:210px;
  height:210px;
  background:radial-gradient(circle, rgba(15,146,116,.17), transparent 72%);
}
.wolfbbs-primer-eyebrow{
  display:inline-flex;
  margin-bottom:9px;
  color:#0f5b9b;
  font-size:.76rem;
  font-weight:800;
  letter-spacing:.09em;
  text-transform:uppercase;
}
.wolfbbs-primer-title{
  display:block;
  margin-bottom:8px;
  color:#113c6a;
  font-size:1.14rem;
}
.wolfbbs-primer p{
  margin:0 0 11px;
  color:#23496f;
}
.wolfbbs-primer ul{
  margin:11px 0 0 18px;
}
.wolfbbs-primer-actions{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  margin-top:12px;
}
.wolfbbs-primer-actions a{
  display:inline-flex;
  align-items:center;
  min-height:34px;
  padding:7px 13px;
  border-radius:var(--radius-pill);
  border:1px solid #c0d3ec;
  background:linear-gradient(180deg,#ffffff,#ebf4ff);
  color:#124783;
  font-size:.88rem;
  font-weight:760;
}
.wolfbbs-primer-actions a:hover{
  background:linear-gradient(180deg,#edf6ff,#dfeeff);
  transform:translateY(-1px);
  text-decoration:none;
}
.wolfbbs-action-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(170px,1fr));
  gap:10px;
  margin-top:12px;
}
.wolfbbs-action-card{
  display:flex;
  flex-direction:column;
  gap:6px;
  padding:15px 16px;
  color:var(--text);
  background:
    radial-gradient(circle at top right, rgba(10,92,198,.14), transparent 44%),
    linear-gradient(180deg,#ffffff,#f5faff);
  transition:transform .18s ease, box-shadow .18s ease, border-color .18s ease;
}
.wolfbbs-action-card:hover{
  text-decoration:none;
  transform:translateY(-2px);
  border-color:var(--line-strong);
  box-shadow:var(--shadow-lg);
}
.wolfbbs-action-card strong{
  color:#103d6d;
  font-size:.99rem;
}
.wolfbbs-action-card span{
  color:var(--muted);
  font-size:.89rem;
}
.wolfbbs-chip-row{
  display:flex;
  flex-wrap:wrap;
  gap:6px;
}
.wolfbbs-chip{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:2px 10px;
  border-radius:var(--radius-pill);
  background:linear-gradient(180deg,#f4f9ff,#e7f0ff);
  border:1px solid #c4d7f0;
  color:#224e85;
  font-size:.8rem;
  font-weight:760;
}
.wolfbbs-inline-form{
  display:flex;
  flex-wrap:wrap;
  align-items:flex-end;
  gap:10px;
}
.wolfbbs-utility-form{
  display:flex;
  flex-wrap:wrap;
  align-items:flex-end;
  gap:10px;
  padding:11px 12px;
  border-radius:12px;
  background:linear-gradient(180deg,rgba(255,255,255,.94),rgba(246,250,255,.92));
}
.wolfbbs-inline-form label{
  margin:0;
}
.wolfbbs-utility-form label{
  margin:0;
}
.wolfbbs-section-nav,.wolfbbs-recent-rail{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  margin:0 0 16px;
}
.wolfbbs-section-nav a,.wolfbbs-recent-rail a{
  display:inline-flex;
  align-items:center;
  min-height:30px;
  padding:6px 12px;
  border-radius:var(--radius-pill);
  background:linear-gradient(180deg,#ffffff,#edf5ff);
  border:1px solid #c6d7ee;
  color:#1e4a80;
  font-size:.84rem;
  font-weight:760;
}
.wolfbbs-section-nav a:hover,.wolfbbs-recent-rail a:hover{
  text-decoration:none;
  background:linear-gradient(180deg,#edf6ff,#dfeeff);
}
.wolfbbs-inline-filter{
  margin:12px 0 10px;
  padding:11px 13px;
  background:linear-gradient(180deg,rgba(255,255,255,.94),rgba(246,251,255,.95));
  border:1px solid var(--line);
  border-radius:var(--radius-sm);
  box-shadow:var(--shadow-sm);
}
.wolfbbs-banner{
  display:flex;
  align-items:flex-start;
  justify-content:space-between;
  gap:12px;
  margin:14px 0;
  padding:14px 16px;
  border-radius:var(--radius-sm);
  border:1px solid var(--line);
  background:linear-gradient(180deg,#fbfdff,#f1f7ff);
  box-shadow:var(--shadow-sm);
}
.wolfbbs-banner strong{
  display:block;
  margin-bottom:4px;
}
.wolfbbs-banner p{
  margin:0;
}
.wolfbbs-banner[data-kind="notice"]{
  border-color:#b9d5be;
  background:linear-gradient(180deg,#f7fff9,#edf9f0);
}
.wolfbbs-banner[data-kind="error"]{
  border-color:#efc1c6;
  background:linear-gradient(180deg,#fff9fa,#fff0f2);
}
.wolfbbs-banner-close{
  border:0;
  min-height:auto;
  padding:4px 8px;
  border-radius:var(--radius-pill);
  background:#e7eef8;
  color:#26486f;
  font-size:.8rem;
  box-shadow:none;
}
.wolfbbs-banner-close:hover{
  background:#d7e6fb;
}
.wolfbbs-filter-summary{
  margin:12px 0 16px;
}
.wolfbbs-filter-summary h2{
  margin-bottom:8px;
}
.wolfbbs-active-filters{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  margin:10px 0 0;
}
.wolfbbs-filter-chip{
  display:inline-flex;
  align-items:center;
  gap:6px;
  min-height:28px;
  padding:4px 10px;
  border-radius:var(--radius-pill);
  border:1px solid #c6d8ef;
  background:#f6fbff;
  color:#244c83;
  font-size:.82rem;
  font-weight:700;
}
.wolfbbs-filter-reset{
  display:inline-flex;
  align-items:center;
  min-height:28px;
  padding:4px 10px;
  border-radius:var(--radius-pill);
  border:1px solid #d3dce8;
  background:#fff;
  color:#37506c;
  font-size:.82rem;
  font-weight:700;
}
.wolfbbs-filter-reset:hover{
  text-decoration:none;
  background:#f5f9fe;
}
.wolfbbs-form-note{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  margin:0 0 10px;
  color:#35506b;
  font-size:.86rem;
}
.wolfbbs-form-note strong{
  color:#17385e;
}
.wolfbbs-form-status{
  display:inline-flex;
  align-items:center;
  min-height:26px;
  padding:3px 10px;
  border-radius:var(--radius-pill);
  background:#edf4ff;
  border:1px solid #c8daf1;
  color:#244c83;
  font-size:.78rem;
  font-weight:700;
}
.wolfbbs-form-status.dirty{
  background:#fff5df;
  border-color:#f0d7a0;
  color:#7b5400;
}
.wolfbbs-form-status.error{
  background:#fff0f2;
  border-color:#efc1c6;
  color:#8b2331;
}
.wolfbbs-form-secondary{
  border:1px solid #c8d7ea;
  background:linear-gradient(180deg,#f9fcff,#eef5ff);
  color:#1f4c84;
  box-shadow:none;
}
.wolfbbs-form-secondary:hover{
  background:linear-gradient(180deg,#eef6ff,#e0edff);
}
.wolfbbs-compose-shell{
  display:flex;
  flex-direction:column;
  gap:10px;
}
.wolfbbs-compose-toolbar{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:8px;
  padding:10px 12px;
  border:1px solid #d2deef;
  border-radius:var(--radius-sm);
  background:linear-gradient(180deg,#f8fbff,#eef5ff);
}
.wolfbbs-compose-toolbar button{
  min-height:30px;
  padding:6px 10px;
  border-radius:var(--radius-pill);
  border:1px solid #c8d7ea;
  background:#fff;
  color:#1f4a80;
  box-shadow:none;
  font-size:.82rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:700;
}
.wolfbbs-compose-toolbar button:hover{
  background:linear-gradient(180deg,#eef6ff,#e2eeff);
}
.wolfbbs-compose-meta{
  margin-left:auto;
  color:#4a5f76;
  font-size:.8rem;
  font-weight:700;
}
.wolfbbs-compose-preview{
  display:none;
  padding:12px 14px;
  border:1px dashed #bfd2eb;
  border-radius:12px;
  background:#fbfdff;
  color:#24384e;
}
.wolfbbs-compose-preview.active{
  display:block;
}
.wolfbbs-compose-preview p:last-child{
  margin-bottom:0;
}
.wolfbbs-compose-help{
  display:none;
  padding:12px 14px;
  border:1px solid #d6e1f0;
  border-radius:12px;
  background:#ffffff;
  color:#28415d;
}
.wolfbbs-compose-help.active{
  display:block;
}
.wolfbbs-compose-help strong{
  display:block;
  margin-bottom:8px;
}
.wolfbbs-compose-help ul{
  margin:0;
  padding-left:18px;
}
.wolfbbs-compose-focus{
  position:relative;
  z-index:3;
}
.wolfbbs-compose-focus textarea{
  min-height:320px;
}
.wolfbbs-compose-focus .wolfbbs-compose-shell{
  padding:12px;
  border:1px solid #c6d8ef;
  border-radius:var(--radius-sm);
  background:#ffffff;
  box-shadow:var(--shadow-lg);
}
body.wolfbbs-compose-fullscreen-open{
  overflow:hidden;
}
.wolfbbs-compose-fullscreen{
  position:fixed;
  inset:16px;
  z-index:82;
  overflow:auto;
  padding:18px;
  border:1px solid #c6d8ef;
  border-radius:20px;
  background:rgba(255,255,255,.98);
  box-shadow:0 28px 72px rgba(10,23,44,.24);
}
.wolfbbs-compose-fullscreen textarea{
  min-height:58vh;
}
.wolfbbs-compose-fullscreen .wolfbbs-compose-shell{
  padding:14px;
  border:1px solid #c6d8ef;
  border-radius:var(--radius-sm);
  background:#ffffff;
}
.wolfbbs-handle-assist{
  display:none;
  flex-wrap:wrap;
  gap:8px;
  padding:10px 12px;
  border:1px solid #d6e1f0;
  border-radius:var(--radius-sm);
  background:#fffdfa;
}
.wolfbbs-handle-assist.active{
  display:flex;
}
.wolfbbs-handle-assist button{
  min-height:30px;
  padding:6px 10px;
  border-radius:var(--radius-pill);
  border:1px solid #c8d7ea;
  background:#fff;
  color:#1f4a80;
  box-shadow:none;
  font-size:.82rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:700;
}
.wolfbbs-handle-assist button:hover,
.wolfbbs-handle-assist button.active{
  background:#11457c;
  color:#fffdfa;
}
.wolfbbs-presence-list{
  display:grid;
  gap:12px;
}
.wolfbbs-presence-card{
  padding:12px 14px;
  border:1px solid #d6e1f0;
  border-radius:var(--radius-sm);
  background:linear-gradient(180deg,#ffffff,#f8fbff);
  box-shadow:var(--shadow-sm);
}
.wolfbbs-presence-card strong{
  display:block;
  color:#17385e;
}
.wolfbbs-presence-card span{
  display:block;
  margin-top:4px;
  color:#56677a;
  font-size:.92rem;
}
.wolfbbs-command-block{
  position:relative;
}
.wolfbbs-copy-button{
  position:absolute;
  top:10px;
  right:10px;
  border:1px solid #cad8eb;
  min-height:auto;
  padding:4px 9px;
  border-radius:var(--radius-pill);
  background:#fff;
  color:#26486f;
  box-shadow:none;
  font-size:.8rem;
}
.wolfbbs-copy-button:hover{
  background:#eef5ff;
}
.wolfbbs-inline-actions{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
}
.wolfbbs-stack{
  display:flex;
  flex-direction:column;
  gap:14px;
  margin:14px 0 18px;
}
.wolfbbs-control-strip{
  display:flex;
  flex-wrap:wrap;
  gap:10px;
  align-items:flex-end;
  margin:12px 0 16px;
}
.wolfbbs-control-strip > *{
  margin:0;
}
.wolfbbs-table-wrap{
  width:100%;
  overflow:auto;
  border-radius:var(--radius-sm);
}
.wolfbbs-table-wrap table{
  min-width:680px;
  margin:0;
}
.wolfbbs-status-pill{
  display:inline-flex;
  align-items:center;
  min-height:26px;
  padding:3px 10px;
  border-radius:var(--radius-pill);
  border:1px solid #c8daf1;
  background:#edf4ff;
  color:#244c83;
  font-size:.78rem;
  font-weight:700;
}
.wolfbbs-status-pill.ok{
  border-color:#b9d5be;
  background:#edf9f0;
  color:#21623e;
}
.wolfbbs-status-pill.warn{
  border-color:#f0d7a0;
  background:#fff5df;
  color:#7b5400;
}
.wolfbbs-status-pill.danger{
  border-color:#efc1c6;
  background:#fff0f2;
  color:#8b2331;
}
.wolfbbs-meta-list{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(180px,1fr));
  gap:10px;
  margin:0;
}
.wolfbbs-meta-list div{
  padding:12px 14px;
  border:1px solid #d8e4f1;
  border-radius:var(--radius-sm);
  background:#fbfdff;
}
.wolfbbs-meta-list dt{
  margin:0 0 4px;
  color:#4b6178;
  font-size:.8rem;
  font-weight:700;
  text-transform:uppercase;
  letter-spacing:.04em;
}
.wolfbbs-meta-list dd{
  margin:0;
  color:#16385f;
  font-weight:700;
}
.wolfbbs-chat-layout{
  display:grid;
  grid-template-columns:minmax(0,2.2fr) minmax(290px,1fr);
  gap:15px;
  margin:14px 0 18px;
}
.wolfbbs-chat-pane{
  height:420px;
  overflow:auto;
  padding:14px 16px;
  font-family:"SFMono-Regular","Menlo","Consolas",monospace;
  white-space:pre-wrap;
  word-break:break-word;
  border-radius:var(--radius-sm);
  border:1px solid #173d67;
  background:
    radial-gradient(circle at 86% -12%, rgba(85,156,236,.2), transparent 44%),
    radial-gradient(circle at 12% 112%, rgba(18,137,108,.13), transparent 40%),
    linear-gradient(180deg,#0f1f33,#091322);
  box-shadow:0 16px 32px rgba(8,14,26,.33);
}
.wolfbbs-chat-line{
  padding:6px 0;
  border-bottom:1px solid rgba(207,225,248,.16);
  border-left:3px solid transparent;
  padding-left:8px;
  border-radius:9px;
}
.wolfbbs-chat-line:last-child{
  border-bottom:0;
}
.wolfbbs-chat-line-self{
  border-left-color:#5ab2ff;
  background:rgba(34,79,129,.26);
}
.wolfbbs-chat-line-system{
  border-left-color:#86ddb0;
  background:rgba(33,78,60,.3);
}
.wolfbbs-chat-line-mention{
  border-left-color:#ffcf73;
  background:rgba(103,77,26,.3);
}
.wolfbbs-chat-line strong{
  color:#f3f8ff;
}
.wolfbbs-chat-line-meta{
  color:#a9c5ea;
  font-size:.8rem;
  font-weight:700;
}
.wolfbbs-chat-line-body{
  color:#dce8fb;
}
.wolfbbs-channel-badges{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  margin:10px 0 0;
}
.wolfbbs-chat-toolbar-meta{
  margin-left:auto;
  display:inline-flex;
  align-items:center;
  gap:8px;
  color:#4a5f76;
  font-size:.82rem;
  font-weight:700;
}
.wolfbbs-chat-status-pill{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:3px 10px;
  border-radius:var(--radius-pill);
  border:1px solid #c8daf1;
  background:#edf4ff;
  color:#244c83;
  text-transform:uppercase;
  letter-spacing:.04em;
  font-size:.72rem;
}
.wolfbbs-chat-status-pill[data-state="live"]{
  background:#edf9f0;
  border-color:#b9d5be;
  color:#21623e;
}
.wolfbbs-chat-status-pill[data-state="ready"]{
  background:#edf4ff;
  border-color:#c8daf1;
  color:#244c83;
}
.wolfbbs-chat-status-pill[data-state="warn"]{
  background:#fff5df;
  border-color:#f0d7a0;
  color:#7b5400;
}
.wolfbbs-chat-status-pill[data-state="error"]{
  background:#fff0f2;
  border-color:#efc1c6;
  color:#8b2331;
}
.wolfbbs-chat-status-pill[data-state="working"]{
  background:#eef5ff;
  border-color:#bfd4ec;
  color:#2d4f7a;
}
.wolfbbs-channel-badges button{
  min-height:30px;
  padding:6px 10px;
  border-radius:var(--radius-pill);
  border:1px solid #c8d7ea;
  background:#fff;
  color:#1f4a80;
  box-shadow:none;
  font-size:.82rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:700;
}
.wolfbbs-channel-badges button:hover{
  background:linear-gradient(180deg,#edf6ff,#dfeeff);
}
.wolfbbs-chat-primer{
  margin:14px 0 18px;
  padding:16px 18px;
  border:1px solid rgba(114,180,232,.34);
  border-radius:22px;
  background:
    radial-gradient(circle at 0% 0%, rgba(134,212,255,.18), transparent 38%),
    radial-gradient(circle at 100% 100%, rgba(111,218,184,.12), transparent 34%),
    linear-gradient(180deg,rgba(16,25,38,.96),rgba(10,17,27,.98));
  box-shadow:0 18px 34px rgba(0,0,0,.28);
  color:#d9ecff;
}
.wolfbbs-chat-primer strong{
  color:#ffffff;
}
.wolfbbs-chat-shell{
  display:grid;
  grid-template-columns:minmax(230px,280px) minmax(0,1fr) minmax(250px,320px);
  gap:16px;
  align-items:start;
  margin:14px 0 18px;
}
.wolfbbs-chat-column{
  display:flex;
  flex-direction:column;
  gap:16px;
  min-width:0;
}
.wolfbbs-chat-side-card,
.wolfbbs-chat-header-card,
.wolfbbs-chat-transcript-card,
.wolfbbs-chat-composer-card{
  margin:0;
}
.wolfbbs-room-list{
  display:flex;
  flex-direction:column;
  gap:10px;
}
.wolfbbs-room-card{
  position:relative;
  padding:14px 14px 12px;
  border:1px solid rgba(95,136,172,.52);
  border-radius:18px;
  background:
    radial-gradient(circle at 100% 0%, rgba(134,212,255,.12), transparent 36%),
    linear-gradient(180deg,rgba(23,36,54,.96),rgba(17,28,42,.98));
  box-shadow:0 10px 22px rgba(0,0,0,.24);
}
.wolfbbs-room-card.active{
  border-color:rgba(134,212,255,.76);
  box-shadow:0 0 0 1px rgba(134,212,255,.18), 0 16px 28px rgba(0,0,0,.34);
}
.wolfbbs-room-card.unread{
  border-left:4px solid #86d4ff;
  padding-left:11px;
}
.wolfbbs-room-card.locked{
  border-color:rgba(242,205,128,.6);
}
.wolfbbs-room-card-head{
  display:flex;
  gap:8px;
  align-items:flex-start;
  justify-content:space-between;
}
.wolfbbs-room-trigger{
  display:inline-flex;
  align-items:center;
  justify-content:flex-start;
  min-height:0;
  padding:0;
  border:0;
  background:none;
  box-shadow:none;
  font:800 .98rem/1.2 "Sora","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  color:#f4fbff;
  text-shadow:none;
}
.wolfbbs-room-trigger:hover{
  transform:none;
  filter:none;
  background:none;
  box-shadow:none;
  color:#9edfff;
}
.wolfbbs-room-card-badges{
  display:flex;
  flex-wrap:wrap;
  gap:6px;
  justify-content:flex-end;
}
.wolfbbs-room-badge{
  display:inline-flex;
  align-items:center;
  min-height:22px;
  padding:2px 8px;
  border-radius:999px;
  border:1px solid rgba(120,171,210,.45);
  background:rgba(21,40,60,.9);
  color:#c9e9ff;
  font-size:.72rem;
  font-weight:700;
  letter-spacing:.03em;
  text-transform:uppercase;
}
.wolfbbs-room-card-meta{
  display:flex;
  flex-wrap:wrap;
  gap:8px 12px;
  margin-top:8px;
  color:#9ab5cb;
  font-size:.8rem;
}
.wolfbbs-room-card-preview{
  margin:9px 0 0;
  color:#dae5f6;
  font-size:.92rem;
  line-height:1.45;
}
.wolfbbs-room-leave{
  margin-top:10px;
  min-height:28px;
  padding:6px 10px;
  border-radius:999px;
  border:1px solid rgba(120,171,210,.42);
  background:rgba(17,31,46,.88);
  box-shadow:none;
  color:#d7f2ff;
  font-size:.78rem;
}
.wolfbbs-room-leave:hover{
  transform:none;
  box-shadow:none;
  background:rgba(25,45,66,.96);
}
.wolfbbs-chat-header-card{
  padding:18px 18px 16px;
  border-radius:24px;
}
.wolfbbs-chat-header-main{
  display:flex;
  gap:16px;
  align-items:flex-start;
  justify-content:space-between;
}
.wolfbbs-chat-kicker{
  margin:0 0 6px;
  color:#8fb8d7;
  font-size:.78rem;
  font-weight:800;
  letter-spacing:.12em;
  text-transform:uppercase;
}
.wolfbbs-chat-header-stats{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  justify-content:flex-end;
  color:#b8d2e9;
  font-size:.84rem;
  font-weight:700;
}
.wolfbbs-chat-toolbar{
  display:grid;
  grid-template-columns:minmax(0,1fr) auto auto auto;
  gap:10px;
  align-items:end;
  margin-top:14px;
}
.wolfbbs-chat-toolbar label{
  margin:0;
}
.wolfbbs-chat-toolbar button{
  min-height:40px;
  padding:10px 14px;
  border-radius:14px;
  border:1px solid rgba(120,171,210,.44);
  background:linear-gradient(180deg,rgba(37,58,81,.96),rgba(26,42,61,.98));
  box-shadow:none;
  color:#eef9ff;
}
.wolfbbs-chat-toolbar button:hover{
  transform:none;
  filter:none;
  box-shadow:none;
  background:linear-gradient(180deg,rgba(49,75,102,.98),rgba(31,50,72,.98));
}
.wolfbbs-chat-transcript-card{
  padding:0;
  overflow:hidden;
}
.wolfbbs-chat-pane{
  min-height:540px;
  height:clamp(420px, 62vh, 760px);
  padding:18px;
  display:flex;
  flex-direction:column;
  gap:12px;
}
.wolfbbs-chat-separator{
  display:flex;
  align-items:center;
  gap:10px;
  color:#8fb8d7;
  font-size:.78rem;
  font-weight:800;
  letter-spacing:.08em;
  text-transform:uppercase;
}
.wolfbbs-chat-separator::before,
.wolfbbs-chat-separator::after{
  content:"";
  flex:1 1 auto;
  height:1px;
  background:linear-gradient(90deg,rgba(134,212,255,.12),rgba(134,212,255,.5),rgba(134,212,255,.12));
}
.wolfbbs-chat-empty{
  margin:auto 0;
  padding:16px 18px;
  border:1px dashed rgba(134,212,255,.26);
  border-radius:18px;
  background:rgba(17,28,42,.54);
  color:#a7bfd6;
}
.wolfbbs-chat-line{
  display:flex;
  gap:12px;
  align-items:flex-start;
  padding:0;
  border:0;
  background:none;
}
.wolfbbs-chat-line-avatar{
  width:34px;
  height:34px;
  flex:0 0 34px;
  display:grid;
  place-items:center;
  border-radius:12px;
  border:1px solid rgba(134,212,255,.24);
  background:linear-gradient(180deg,rgba(29,52,76,.96),rgba(18,33,49,.98));
  color:#dff6ff;
  font-size:.86rem;
  font-weight:800;
}
.wolfbbs-chat-line-bubble{
  flex:1 1 auto;
  min-width:0;
  padding:12px 14px;
  border:1px solid rgba(86,121,153,.34);
  border-radius:18px;
  background:linear-gradient(180deg,rgba(18,31,45,.95),rgba(13,24,36,.98));
  box-shadow:0 10px 18px rgba(0,0,0,.2);
}
.wolfbbs-chat-line-head{
  display:flex;
  gap:10px;
  align-items:baseline;
  justify-content:space-between;
  margin-bottom:6px;
}
.wolfbbs-chat-line strong{
  color:#f4fbff;
}
.wolfbbs-chat-line-meta{
  color:#96b3cd;
  font-size:.77rem;
  font-weight:700;
  white-space:nowrap;
}
.wolfbbs-chat-line-body{
  color:#dce8fb;
  line-height:1.5;
}
.wolfbbs-chat-line-self .wolfbbs-chat-line-bubble{
  border-color:rgba(134,212,255,.44);
  background:linear-gradient(180deg,rgba(24,47,71,.95),rgba(17,33,50,.99));
}
.wolfbbs-chat-line-system .wolfbbs-chat-line-bubble{
  border-color:rgba(111,218,184,.42);
  background:linear-gradient(180deg,rgba(21,50,42,.94),rgba(16,37,31,.98));
}
.wolfbbs-chat-line-mention .wolfbbs-chat-line-bubble{
  border-color:rgba(242,205,128,.48);
  background:linear-gradient(180deg,rgba(70,56,28,.9),rgba(43,34,18,.98));
}
.wolfbbs-chat-composer-card{
  padding:16px 18px;
}
.wolfbbs-chat-composer-form{
  display:grid;
  grid-template-columns:minmax(0,1fr) auto;
  gap:12px;
  margin:0;
  padding:0;
  border:0;
  background:none;
  box-shadow:none;
}
.wolfbbs-chat-composer-field{
  margin:0;
}
.wolfbbs-chat-composer-field input{
  width:100%;
}
#chatStatus{
  margin:0;
  padding:12px 18px 16px;
  color:#9edfff !important;
  font-family:"SFMono-Regular","Menlo","Consolas",monospace;
  font-size:.84rem;
}
.wolfbbs-chat-room-desk{
  display:grid;
  grid-template-columns:1fr;
  gap:8px;
  margin-top:12px;
}
.wolfbbs-chat-room-desk-row{
  display:grid;
  gap:4px;
  padding:11px 12px;
  border:1px solid rgba(86,121,153,.3);
  border-radius:14px;
  background:rgba(13,24,36,.62);
}
.wolfbbs-chat-room-desk-row strong{
  color:#dff6ff;
  font-size:.78rem;
  letter-spacing:.06em;
  text-transform:uppercase;
}
.wolfbbs-chat-room-desk-row span{
  color:#cad8ea;
  line-height:1.45;
}
.wolfbbs-chat-shell.compact{
  grid-template-columns:minmax(220px,250px) minmax(0,1fr);
}
.wolfbbs-chat-shell.compact .wolfbbs-chat-column:last-child{
  display:none;
}
.wolfbbs-chat-shell.compact .wolfbbs-chat-pane{
  min-height:420px;
  height:clamp(360px, 56vh, 620px);
}
.wolfbbs-chat-shell.compact .wolfbbs-chat-line-avatar{
  display:none;
}
.wolfbbs-chat-shell.compact .wolfbbs-chat-line{
  gap:0;
}
.wolfbbs-chat-shell.compact .wolfbbs-chat-line-bubble{
  border-radius:14px;
}
.wolfbbs-split{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(260px,1fr));
  gap:14px;
  margin:14px 0 18px;
}
.wolfbbs-list-clean{
  list-style:none;
  padding:0;
  margin:0;
}
.wolfbbs-list-clean li{
  padding:8px 0;
  border-bottom:1px solid #e6eef8;
}
.wolfbbs-list-clean li:last-child{
  border-bottom:0;
}
.wolfbbs-helper-grid{
  display:grid;
  grid-template-columns:repeat(auto-fit,minmax(200px,1fr));
  gap:10px;
  margin:12px 0 16px;
}
.wolfbbs-helper-card{
  padding:14px 16px;
  border-radius:var(--radius-sm);
  border:1px solid var(--line);
  background:
    radial-gradient(circle at 106% -24%, rgba(10,92,198,.11), transparent 46%),
    linear-gradient(180deg,#ffffff,#f5f9ff);
  box-shadow:var(--shadow-sm);
}
.wolfbbs-helper-card strong{
  display:block;
  margin-bottom:6px;
  color:#17385e;
}
.wolfbbs-helper-card p{
  margin:0;
  color:#4b6178;
}
.wolfbbs-submit-busy{
  opacity:.72;
  pointer-events:none;
}
#chat{
  border:1px solid #173d67 !important;
  border-radius:var(--radius-sm);
  color:#dce8fb;
  box-shadow:0 16px 32px rgba(8,14,26,.33);
}
#chatStatus{
  color:#22568f !important;
  font-weight:680;
  margin:10px 0;
}
#mod{
  margin-top:12px;
}
#mod form{
  background:#f7fbff;
  border-color:#bfd4ec;
}
#wolfbbsCommandButton{
  position:fixed;
  right:20px;
  bottom:18px;
  z-index:40;
  min-height:44px;
  padding:10px 15px;
  border-radius:var(--radius-pill);
  border:1px solid rgba(255,255,255,.42);
  background:linear-gradient(130deg,#0a66d6,#07469a 58%,#0f9274) !important;
  box-shadow:0 18px 36px rgba(9,57,122,.26);
}
#wolfbbsPaletteOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.52);
  display:none;
  z-index:60;
  padding:24px 16px;
}
#wolfbbsPaletteOverlay.active{display:block}
#wolfbbsPalette{
  max-width:760px;
  margin:0 auto;
  background:linear-gradient(180deg,#ffffff,#f4f9ff);
  border:1px solid var(--line);
  border-radius:20px;
  box-shadow:0 28px 68px rgba(8,19,36,.34);
  overflow:hidden;
}
#wolfbbsPaletteHeader{
  padding:14px;
  background:linear-gradient(180deg,#f8fcff,#ebf3ff);
  border-bottom:1px solid var(--line);
}
#wolfbbsPaletteList{
  max-height:min(60vh,520px);
  overflow:auto;
  padding:8px;
}
.wolfbbs-palette-item{
  display:flex;
  justify-content:space-between;
  gap:12px;
  padding:11px 12px;
  border-radius:var(--radius-sm);
  color:var(--text);
}
.wolfbbs-palette-item:hover{
  background:linear-gradient(180deg,#eef6ff,#e0edff);
  text-decoration:none;
}
.wolfbbs-palette-meta{
  color:var(--muted);
  font-size:.82rem;
}
input[type=checkbox],input[type=radio]{
  accent-color:var(--accent);
}
.wolfbbs-chat-sidebar{
  align-self:start;
}
body > h1{
  font-size:clamp(1.7rem,2.45vw,2.35rem);
  background:linear-gradient(120deg,#0b3f88 0%,#0a63ca 56%,#0f9274 100%);
  -webkit-background-clip:text;
  background-clip:text;
  color:transparent;
  -webkit-text-fill-color:transparent;
}
body p,
body li{
  color:#27435f;
}
table,
form,
.wolfbbs-card,
.wolfbbs-helper-card,
.wolfbbs-kpi-card,
.wolfbbs-action-card{
  backdrop-filter:blur(3px);
}
.wolfbbs-card{
  border-top:3px solid rgba(10,92,198,.34);
}
.wolfbbs-kpi-card{
  border-top:3px solid rgba(15,146,116,.52);
}
.wolfbbs-helper-card{
  border-top:3px solid rgba(232,157,34,.45);
}
p.wolfbbs-nav-row a,body > p:has(> a) a{
  box-shadow:0 4px 10px rgba(10,54,112,.09);
}
p.wolfbbs-nav-row a:hover,body > p:has(> a) a:hover{
  box-shadow:0 8px 16px rgba(10,66,138,.17);
}
button,input[type=submit],input[type=button]{
  letter-spacing:.01em;
}
.wolfbbs-action-card{
  border-top:3px solid rgba(10,92,198,.38);
}
.wolfbbs-action-card strong{
  letter-spacing:.01em;
}
.wolfbbs-meta-list div{
  border-left:3px solid rgba(10,92,198,.3);
}
.wolfbbs-chat-pane{
  border-top:3px solid rgba(86,177,255,.65);
}
#wolfbbsCommandButton{
  border:1px solid rgba(255,255,255,.55);
}
body{
  position:relative;
  overflow-x:hidden;
}
body::after{
  content:"";
  position:fixed;
  z-index:-1;
  pointer-events:none;
  inset:auto auto -130px -120px;
  width:360px;
  height:360px;
  opacity:.22;
  background:
    radial-gradient(circle at 34% 34%, rgba(10,92,198,.55), rgba(10,92,198,0) 72%),
    radial-gradient(circle at 70% 68%, rgba(15,146,116,.58), rgba(15,146,116,0) 74%);
  filter:blur(8px);
}
body > h1:first-of-type{
  position:relative;
  display:inline-block;
  padding-right:14px;
}
body > h1:first-of-type::after{
  content:"";
  display:block;
  width:66%;
  height:4px;
  margin-top:10px;
  border-radius:999px;
  background:linear-gradient(90deg, rgba(10,92,198,.72), rgba(15,146,116,.86));
  box-shadow:0 6px 16px rgba(13,92,186,.28);
}
body[data-route^="/admin"] > h1{
  background:linear-gradient(120deg,#15543f 0%,#0a63ca 44%,#09396e 100%);
  -webkit-background-clip:text;
  background-clip:text;
  color:transparent;
  -webkit-text-fill-color:transparent;
}
p.wolfbbs-nav-row,body > p:has(> a){
  position:relative;
  overflow:hidden;
  border-color:#bfd0e8;
  box-shadow:0 10px 18px rgba(9,53,113,.09);
}
p.wolfbbs-nav-row::before,body > p:has(> a)::before{
  content:"";
  position:absolute;
  inset:0;
  pointer-events:none;
  background:
    radial-gradient(circle at 100% 0%, rgba(10,92,198,.14), transparent 45%),
    linear-gradient(100deg, rgba(15,146,116,.08), rgba(10,92,198,0) 42%);
}
p.wolfbbs-nav-row a,body > p:has(> a) a{
  position:relative;
  isolation:isolate;
  transition:transform .16s ease, box-shadow .2s ease, border-color .2s ease, color .2s ease, background .2s ease;
}
p.wolfbbs-nav-row a.wolfbbs-nav-active,
body > p:has(> a) a.wolfbbs-nav-active{
  border-color:#0f5cbf;
  background:linear-gradient(180deg,#1670df,#0a4faa 62%,#0f9274);
  color:#f8fbff;
  box-shadow:0 12px 24px rgba(10,74,152,.3);
}
p.wolfbbs-nav-row[data-wolfbbs-nav-level="secondary"],body > p:has(> a)[data-wolfbbs-nav-level="secondary"]{
  padding:7px 0 2px;
  background:transparent;
  border:0;
  box-shadow:none;
  backdrop-filter:none;
}
p.wolfbbs-nav-row[data-wolfbbs-nav-level="secondary"]::before,body > p:has(> a)[data-wolfbbs-nav-level="secondary"]::before{
  display:none;
}
p.wolfbbs-nav-row[data-wolfbbs-nav-level="secondary"] a,body > p:has(> a)[data-wolfbbs-nav-level="secondary"] a{
  min-height:28px;
  padding:4px 11px;
  background:rgba(247,251,255,.88);
  box-shadow:none;
}
p.wolfbbs-nav-row a.wolfbbs-nav-active:hover,
body > p:has(> a) a.wolfbbs-nav-active:hover{
  filter:brightness(1.03);
}
form{
  position:relative;
  overflow:hidden;
}
form::before{
  content:"";
  position:absolute;
  pointer-events:none;
  inset:0;
  background:radial-gradient(circle at 100% 0%, rgba(10,92,198,.08), transparent 42%);
}
form > *{
  position:relative;
  z-index:1;
}
input[type=text],input[type=password],input[type=email],input[type=number],input[type=url],input[type=search],select,textarea{
  border-color:#b2c7e0;
  background:linear-gradient(180deg,#ffffff,#f9fcff);
}
select{
  appearance:none;
  background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='14' height='14' viewBox='0 0 24 24' fill='none' stroke='%23315579' stroke-width='2.2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E"), linear-gradient(180deg,#ffffff,#f9fcff);
  background-repeat:no-repeat, no-repeat;
  background-position:right 10px center, center;
  background-size:14px 14px, auto;
  padding-right:34px;
}
textarea{
  line-height:1.45;
}
button,input[type=submit],input[type=button]{
  position:relative;
  overflow:hidden;
}
button::after,input[type=submit]::after,input[type=button]::after{
  content:"";
  position:absolute;
  inset:0;
  pointer-events:none;
  background:linear-gradient(110deg, rgba(255,255,255,.2), rgba(255,255,255,0) 44%);
}
.wolfbbs-kpi-card strong{
  display:inline-block;
  background:linear-gradient(108deg,#0f355f,#0a63ca 58%,#0f9274);
  -webkit-background-clip:text;
  background-clip:text;
  color:transparent;
  -webkit-text-fill-color:transparent;
}
table,
form,
.wolfbbs-card,
.wolfbbs-helper-card,
.wolfbbs-kpi-card,
.wolfbbs-action-card,
.wolfbbs-banner{
  transition:transform .2s ease, box-shadow .2s ease, border-color .2s ease;
}
.wolfbbs-card:hover,
.wolfbbs-helper-card:hover,
.wolfbbs-action-card:hover,
.wolfbbs-kpi-card:hover{
  transform:translateY(-2px);
  box-shadow:0 20px 34px rgba(11,34,66,.16);
}
.wolfbbs-chat-pane{
  background:
    radial-gradient(circle at 86% -12%, rgba(111,179,255,.2), transparent 44%),
    radial-gradient(circle at 12% 112%, rgba(40,188,148,.2), transparent 42%),
    linear-gradient(180deg,#101d31,#091120);
}
.wolfbbs-chat-line{
  transition:background .16s ease, border-color .16s ease;
}
.wolfbbs-chat-line:hover{
  background:rgba(49,89,136,.22);
}
#wolfbbsPalette{
  border-color:#b8cae4;
}
#wolfbbsPaletteHeader{
  position:sticky;
  top:0;
  z-index:2;
}
*::-webkit-scrollbar{
  width:10px;
  height:10px;
}
*::-webkit-scrollbar-track{
  background:rgba(160,183,210,.22);
}
*::-webkit-scrollbar-thumb{
  border-radius:999px;
  border:2px solid rgba(245,249,255,.75);
  background:linear-gradient(180deg,#87b0e7,#5e8fcc);
}
*::-webkit-scrollbar-thumb:hover{
  background:linear-gradient(180deg,#739fdd,#4d7fbd);
}
/* UX pass: cleaner structure and less generated-feeling chrome */
body{
  width:min(1180px,calc(100% - 2rem));
  padding:18px 0 90px;
  background:
    radial-gradient(900px 320px at 8% -14%, rgba(10,92,198,.11), transparent 66%),
    radial-gradient(860px 260px at 96% -14%, rgba(15,146,116,.08), transparent 64%),
    linear-gradient(180deg,#f6f9fc,#edf3fa);
}
body::before{
  opacity:.18;
}
body::after{
  display:none;
}
body > h1{
  background:none;
  color:#12314f;
  -webkit-text-fill-color:currentColor;
}
body > h1:first-of-type{
  display:block;
  padding-right:0;
  margin-bottom:6px;
}
body > h1:first-of-type::after{
  display:none;
}
body p,
body li{
  color:#2b445f;
}
p.wolfbbs-nav-row,body > p:has(> a){
  position:sticky;
  top:10px;
  z-index:34;
  backdrop-filter:saturate(140%) blur(8px);
  background:rgba(255,255,255,.88);
  border-color:#c4d3e5;
  box-shadow:0 8px 20px rgba(11,45,91,.11);
}
p.wolfbbs-nav-row::before,body > p:has(> a)::before{
  background:linear-gradient(90deg, rgba(10,92,198,.06), rgba(15,146,116,.05));
}
p.wolfbbs-nav-row a,body > p:has(> a) a{
  border-color:#c8d5e4;
  background:#f8fbff;
  color:#21496f;
  box-shadow:none;
}
p.wolfbbs-nav-row a:hover,body > p:has(> a) a:hover{
  background:#eef4fb;
  box-shadow:none;
}
.wolfbbs-nav-more{
  position:relative;
}
.wolfbbs-nav-more > summary{
  display:inline-flex;
  align-items:center;
  min-height:30px;
  padding:6px 12px;
  border-radius:var(--radius-pill);
  background:#f8fbff;
  border:1px solid #c8d5e4;
  color:#21496f;
  cursor:pointer;
  list-style:none;
  font-size:.84rem;
  font-weight:760;
}
.wolfbbs-nav-more > summary::-webkit-details-marker{
  display:none;
}
.wolfbbs-nav-more[open] > summary{
  background:#eef4fb;
}
.wolfbbs-nav-more-panel{
  position:absolute;
  top:calc(100% + 8px);
  left:0;
  z-index:45;
  display:grid;
  gap:8px;
  width:min(320px,calc(100vw - 2rem));
  padding:12px;
  border:1px solid #c8d7ea;
  border-radius:16px;
  background:#fcfeff;
  box-shadow:0 24px 44px rgba(9,41,81,.18);
}
.wolfbbs-nav-more-panel a{
  display:flex;
  width:100%;
  justify-content:flex-start;
}
p.wolfbbs-nav-row a.wolfbbs-nav-active,
body > p:has(> a) a.wolfbbs-nav-active{
  background:#0f4f93;
  border-color:#0f4f93;
  color:#f4f9ff;
  box-shadow:0 8px 16px rgba(9,61,118,.22);
}
.wolfbbs-page-hero{
  display:flex;
  justify-content:space-between;
  align-items:flex-start;
  gap:14px;
  margin:0 0 10px;
  padding:12px 14px;
  border:1px solid #cbd7e6;
  border-radius:12px;
  background:linear-gradient(180deg,#ffffff,#f7fbff);
  box-shadow:0 8px 18px rgba(13,41,74,.08);
}
.wolfbbs-page-hero-main{
  min-width:0;
}
.wolfbbs-page-hero-main p{
  margin:4px 0 0;
  color:#425f7b;
}
.wolfbbs-page-hero-meta{
  display:flex;
  flex-wrap:wrap;
  gap:6px 10px;
  justify-content:flex-end;
  align-items:flex-start;
}
.wolfbbs-hero-chip{
  display:inline-flex;
  align-items:center;
  min-height:26px;
  padding:3px 10px;
  border-radius:999px;
  border:1px solid #c8d6e7;
  background:#f3f8ff;
  color:#1d456b;
  font-size:.76rem;
  font-weight:760;
  text-transform:uppercase;
  letter-spacing:.05em;
}
.wolfbbs-hero-chip[data-kind="admin"]{
  background:#eaf6ef;
  border-color:#bddac8;
  color:#1f6144;
}
.wolfbbs-hero-chip[data-kind="caller"]{
  background:#eef4ff;
  border-color:#c5d5ec;
  color:#244f80;
}
.wolfbbs-hero-chip[data-kind="guest"]{
  background:#fff6e8;
  border-color:#efd4ac;
  color:#7d5206;
}
.wolfbbs-main{
  display:flex;
  flex-direction:column;
  gap:15px;
  margin-top:14px;
}
.wolfbbs-main > section,
.wolfbbs-main > article{
  scroll-margin-top:120px;
}
.wolfbbs-main .wolfbbs-grid,
.wolfbbs-main .wolfbbs-stack,
.wolfbbs-main .wolfbbs-chat-layout,
.wolfbbs-main .wolfbbs-split{
  margin:0;
}
.wolfbbs-main section + section{
  margin-top:2px;
}
.wolfbbs-main > section > h2:first-child,
.wolfbbs-main > article > h2:first-child{
  position:relative;
  display:flex;
  align-items:center;
  gap:8px;
  padding-bottom:7px;
  margin-bottom:11px;
  border-bottom:1px solid #d6e2ef;
  color:#163b60;
}
.wolfbbs-main > section > h2:first-child::before,
.wolfbbs-main > article > h2:first-child::before{
  content:"";
  width:8px;
  height:8px;
  border-radius:50%;
  background:linear-gradient(180deg,#0f64cb,#0f9274);
  box-shadow:0 0 0 4px rgba(15,100,203,.1);
}
.wolfbbs-dashboard{
  display:grid;
  grid-template-columns:minmax(0,1.8fr) minmax(0,1.2fr) minmax(0,1fr);
  gap:12px;
  margin:0 0 2px;
}
.wolfbbs-dashboard-card{
  position:relative;
  overflow:hidden;
  padding:13px 14px;
  border:1px solid #cad8e8;
  border-radius:13px;
  background:
    radial-gradient(circle at 110% -20%, rgba(15,100,203,.12), transparent 42%),
    linear-gradient(180deg,#ffffff,#f6faff);
  box-shadow:0 8px 18px rgba(8,38,76,.09);
}
.wolfbbs-dashboard-card h3{
  margin:0 0 4px;
  font-size:1.01rem;
  color:#143b63;
}
.wolfbbs-dashboard-sub{
  margin:0 0 10px;
  color:#496786;
  font-size:.84rem;
}
.wolfbbs-chart-shell{
  position:relative;
  height:170px;
  margin:0 0 10px;
  border:1px solid #d7e3f0;
  border-radius:11px;
  background:
    linear-gradient(180deg,rgba(250,253,255,.95),rgba(243,249,255,.95));
}
.wolfbbs-chart-canvas{
  width:100%;
  height:100%;
  display:block;
}
.wolfbbs-chart-legend{
  display:flex;
  flex-wrap:wrap;
  gap:7px;
}
.wolfbbs-chart-legend span{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:2px 9px;
  border-radius:999px;
  border:1px solid #c9d7e8;
  background:#f2f8ff;
  color:#234c76;
  font-size:.78rem;
  font-weight:700;
}
.wolfbbs-dash-metrics{
  display:grid;
  grid-template-columns:1fr;
  gap:8px;
}
.wolfbbs-dash-metric{
  display:flex;
  justify-content:space-between;
  align-items:center;
  gap:8px;
  padding:8px 10px;
  border:1px solid #d5e1ef;
  border-radius:10px;
  background:#fbfdff;
}
.wolfbbs-dash-metric label{
  margin:0;
  display:block;
  color:#4a6580;
  font-size:.79rem;
  font-weight:700;
}
.wolfbbs-dash-metric strong{
  color:#143d65;
  font-size:1rem;
}
.wolfbbs-dash-mini{
  display:grid;
  grid-template-columns:repeat(2,minmax(0,1fr));
  gap:7px;
}
.wolfbbs-dash-chip{
  min-height:56px;
  display:flex;
  flex-direction:column;
  justify-content:center;
  padding:8px 10px;
  border-radius:10px;
  border:1px solid #cfdeed;
  background:#f5faff;
}
.wolfbbs-dash-chip strong{
  color:#143d66;
  font-size:1.04rem;
  line-height:1.05;
}
.wolfbbs-dash-chip span{
  color:#56728d;
  font-size:.77rem;
  text-transform:uppercase;
  letter-spacing:.05em;
}
.wolfbbs-guide-strip{
  display:grid;
  grid-template-columns:minmax(0,1fr) auto;
  gap:10px 14px;
  align-items:start;
  margin:8px 0 12px;
  padding:12px 14px;
  border:1px solid #c9d7e8;
  border-left:4px solid #0f5ba8;
  border-radius:12px;
  background:linear-gradient(180deg,#ffffff,#f5f9ff);
}
.wolfbbs-guide-strip strong{
  display:block;
  margin-bottom:3px;
  color:#143c63;
}
.wolfbbs-guide-strip p{
  margin:0;
  color:#3f5e7b;
}
.wolfbbs-guide-actions{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  justify-content:flex-end;
}
.wolfbbs-guide-actions a{
  display:inline-flex;
  align-items:center;
  min-height:30px;
  padding:6px 11px;
  border-radius:999px;
  border:1px solid #c6d5e6;
  background:#f8fbff;
  color:#214d7a;
  font-size:.83rem;
  font-weight:720;
}
.wolfbbs-guide-actions a:hover{
  background:#edf4fc;
  text-decoration:none;
}
.wolfbbs-guide-details{
  grid-column:1 / -1;
}
.wolfbbs-guide-details summary{
  cursor:pointer;
  color:#3a5773;
  font-size:.84rem;
  font-weight:640;
}
.wolfbbs-guide-details ul{
  margin:8px 0 0 18px;
}
.wolfbbs-guide-details li{
  color:#4a6580;
}
.wolfbbs-section-nav{
  position:sticky;
  top:60px;
  z-index:30;
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:8px;
  margin:8px 0 12px;
  padding:8px 10px;
  background:rgba(248,252,255,.93);
  border:1px solid #ccd9e8;
  border-radius:11px;
  box-shadow:0 6px 14px rgba(9,46,94,.08);
  overflow:auto hidden;
  white-space:nowrap;
}
.wolfbbs-section-nav-minimal{
  gap:7px;
  padding:9px 12px;
}
.wolfbbs-section-nav-label{
  color:#5b7390;
  font-size:.74rem;
  font-weight:800;
  letter-spacing:.06em;
  text-transform:uppercase;
}
.wolfbbs-section-nav a{
  background:#f7fbff;
}
.wolfbbs-section-nav-tools{
  position:relative;
}
.wolfbbs-section-nav-tools > summary{
  display:inline-flex;
  align-items:center;
  min-height:28px;
  padding:.45rem .8rem;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#f7fbff;
  color:#1f4a78;
  cursor:pointer;
  list-style:none;
  font-size:.8rem;
  font-weight:760;
}
.wolfbbs-section-nav-tools > summary::-webkit-details-marker{
  display:none;
}
.wolfbbs-section-nav-tools[open] > summary{
  background:#eaf2fb;
}
.wolfbbs-section-nav-tools-panel{
  position:absolute;
  top:calc(100% + 8px);
  left:0;
  z-index:42;
  display:grid;
  gap:8px;
  width:min(280px,calc(100vw - 2rem));
  padding:11px;
  border:1px solid #c8d7ea;
  border-radius:16px;
  background:#fcfeff;
  box-shadow:0 24px 44px rgba(9,41,81,.18);
}
.wolfbbs-section-nav-tools-panel input[type="search"]{
  width:100%;
}
.wolfbbs-section-nav-tools-panel button{
  width:100%;
  justify-content:flex-start;
}
.wolfbbs-section-nav-tool-button{
  min-height:30px;
}
.wolfbbs-card,
.wolfbbs-helper-card,
.wolfbbs-kpi-card,
.wolfbbs-action-card,
form,
table{
  border-color:#d0dcea;
  box-shadow:0 7px 14px rgba(9,39,79,.08);
}
.wolfbbs-card:hover,
.wolfbbs-helper-card:hover,
.wolfbbs-action-card:hover,
.wolfbbs-kpi-card:hover{
  transform:none;
  box-shadow:0 10px 20px rgba(9,39,79,.1);
}
.wolfbbs-card{
  border-top:2px solid rgba(18,79,138,.2);
  background:#ffffff;
}
.wolfbbs-action-card{
  border-top:2px solid rgba(18,79,138,.28);
  background:#ffffff;
}
.wolfbbs-helper-card{
  border-top:2px solid rgba(141,98,22,.34);
}
.wolfbbs-kpi-card{
  border-top:2px solid rgba(28,129,90,.38);
}
#wolfbbsCommandButton{
  min-height:40px;
  padding:8px 13px;
  font-size:.86rem;
}
:root{
  --font-scale:1;
}
body{
  font-size:calc(15px * var(--font-scale));
}
body[data-theme-mode="contrast"]{
  --bg:#ffffff;
  --bg-alt:#f5f7fa;
  --surface:#ffffff;
  --surface-2:#fbfdff;
  --surface-3:#f2f6fb;
  --text:#0d1520;
  --muted:#25384f;
  --line:#b5c5da;
  --line-strong:#7e9dc0;
  --accent:#0a4d9c;
  --accent-strong:#083973;
  --accent-soft:#d8e7ff;
  --shadow-sm:0 6px 14px rgba(12,27,50,.11);
  --shadow:0 12px 30px rgba(12,27,50,.16);
  --shadow-lg:0 18px 40px rgba(10,23,44,.2);
}
body[data-theme-mode="night"]{
  --bg:#0b1523;
  --bg-alt:#101d2e;
  --surface:#132339;
  --surface-2:#162a42;
  --surface-3:#1a3049;
  --text:#e7f1ff;
  --muted:#a9bfd9;
  --line:#2a3f58;
  --line-strong:#3f638b;
  --accent:#5aa7ff;
  --accent-strong:#7bb8ff;
  --accent-soft:#23405f;
  --shadow-sm:0 8px 16px rgba(0,0,0,.26);
  --shadow:0 14px 34px rgba(0,0,0,.34);
  --shadow-lg:0 26px 60px rgba(0,0,0,.42);
}
body[data-theme-mode="night"] .wolfbbs-section-nav-tools > summary,
body[data-theme-mode="night"] .wolfbbs-pref-menu > summary{
  background:linear-gradient(180deg,rgba(29,43,64,.96),rgba(15,24,38,.96));
  border-color:rgba(120,168,222,.28);
  color:#e7f3ff;
}
body[data-theme-mode="night"] .wolfbbs-section-nav-tools[open] > summary,
body[data-theme-mode="night"] .wolfbbs-pref-menu[open] > summary{
  background:linear-gradient(180deg,rgba(43,63,92,.98),rgba(21,33,52,.98));
}
body[data-theme-mode="night"] .wolfbbs-section-nav-tools-panel{
  background:linear-gradient(180deg,rgba(16,24,38,.98),rgba(9,14,23,.98));
  border-color:rgba(120,168,222,.24);
  box-shadow:0 28px 52px rgba(0,0,0,.42);
}
body[data-theme-mode="night"] p,
body[data-theme-mode="night"] li{
  color:#c8d9ef;
}
body[data-theme-mode="night"] h1,
body[data-theme-mode="night"] h2,
body[data-theme-mode="night"] h3{
  color:#edf5ff;
}
body[data-theme-mode="night"] p.wolfbbs-nav-row,
body[data-theme-mode="night"] body > p:has(> a),
body[data-theme-mode="night"] .wolfbbs-page-hero,
body[data-theme-mode="night"] .wolfbbs-guide-strip,
body[data-theme-mode="night"] .wolfbbs-section-nav{
  background:rgba(20,35,55,.95);
}
body[data-theme-mode="night"] input[type=text],
body[data-theme-mode="night"] input[type=password],
body[data-theme-mode="night"] input[type=email],
body[data-theme-mode="night"] input[type=number],
body[data-theme-mode="night"] input[type=url],
body[data-theme-mode="night"] input[type=search],
body[data-theme-mode="night"] select,
body[data-theme-mode="night"] textarea{
  background:linear-gradient(180deg,#162a42,#1b314d);
  color:#e5efff;
  border-color:#365175;
}
body[data-theme-mode="night"] table,
body[data-theme-mode="night"] form,
body[data-theme-mode="night"] .wolfbbs-card,
body[data-theme-mode="night"] .wolfbbs-kpi-card,
body[data-theme-mode="night"] .wolfbbs-action-card,
body[data-theme-mode="night"] .wolfbbs-helper-card{
  background:linear-gradient(180deg,#14263c,#182d46);
}
body[data-density="compact"]{
  --radius:12px;
  --radius-sm:9px;
}
body[data-density="compact"] .wolfbbs-main{
  gap:11px;
}
body[data-density="compact"] .wolfbbs-card,
body[data-density="compact"] .wolfbbs-helper-card,
body[data-density="compact"] .wolfbbs-kpi-card,
body[data-density="compact"] form{
  padding:12px 13px;
}
body[data-density="compact"] .wolfbbs-grid{
  gap:11px;
}
body[data-density="compact"] th,
body[data-density="compact"] td{
  padding:8px 9px;
}
.wolfbbs-skip-link{
  position:fixed;
  top:10px;
  left:10px;
  z-index:92;
  padding:7px 11px;
  border-radius:999px;
  border:1px solid #b8cbe3;
  background:#ffffff;
  color:#0f3f76;
  font-weight:760;
  transform:translateY(-56px);
  transition:transform .14s ease;
}
.wolfbbs-skip-link:focus{
  transform:translateY(0);
}
#wolfbbsScrollProgress{
  position:fixed;
  top:0;
  left:0;
  width:100%;
  height:4px;
  z-index:80;
  background:rgba(159,181,206,.2);
}
#wolfbbsScrollProgress span{
  display:block;
  width:0%;
  height:100%;
  background:linear-gradient(90deg,#0a66d6,#0f9274);
  transition:width .08s linear;
}
.wolfbbs-breadcrumbs{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:6px;
  margin:0 0 8px;
  color:#425d79;
  font-size:.82rem;
  font-weight:660;
}
.wolfbbs-breadcrumbs a{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:2px 9px;
  border-radius:999px;
  border:1px solid #cad9ea;
  background:#f7fbff;
  color:#23507f;
}
.wolfbbs-breadcrumbs a:hover{
  background:#edf4fc;
  text-decoration:none;
}
.wolfbbs-breadcrumb-sep{
  color:#7b91aa;
}
.wolfbbs-pref-controls{
  display:flex;
  flex-wrap:wrap;
  align-items:flex-start;
  justify-content:flex-end;
  gap:6px;
  margin-left:auto;
  max-width:min(100%,38rem);
}
.wolfbbs-status-grid{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  margin-bottom:4px;
}
.wolfbbs-status-grid .wolfbbs-hero-chip,
.wolfbbs-status-grid .wolfbbs-focus-pill{
  min-height:28px;
}
.wolfbbs-pref-controls > button,
.wolfbbs-pref-menu > summary{
  min-height:26px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#f7fbff;
  color:#1f4a78;
  box-shadow:none;
  font-size:.78rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:760;
}
.wolfbbs-pref-controls > button:hover,
.wolfbbs-pref-menu > summary:hover{
  background:#eaf2fb;
}
.wolfbbs-pref-menu{
  position:relative;
}
.wolfbbs-pref-menu > summary{
  display:inline-flex;
  align-items:center;
  list-style:none;
  cursor:pointer;
  white-space:nowrap;
}
.wolfbbs-pref-menu > summary::-webkit-details-marker{
  display:none;
}
.wolfbbs-pref-menu[open] > summary{
  background:#e2efff;
}
.wolfbbs-pref-menu-panel{
  position:absolute;
  top:calc(100% + 8px);
  right:0;
  z-index:54;
  display:grid;
  gap:8px;
  width:min(360px,calc(100vw - 2rem));
  max-height:min(72vh,640px);
  overflow:auto;
  padding:11px;
  border:1px solid #c8d7ea;
  border-radius:18px;
  background:#fcfeff;
  box-shadow:0 24px 44px rgba(9,41,81,.18);
}
.wolfbbs-pref-menu-group{
  display:grid;
  gap:8px;
  padding-top:4px;
}
.wolfbbs-pref-menu-group + .wolfbbs-pref-menu-group{
  margin-top:4px;
  padding-top:10px;
  border-top:1px solid rgba(198,214,232,.7);
}
.wolfbbs-pref-menu-group-title{
  color:#5d7693;
  font-size:.72rem;
  font-weight:800;
  letter-spacing:.06em;
  text-transform:uppercase;
}
.wolfbbs-header-command{
  position:static !important;
  left:auto !important;
  right:auto !important;
  bottom:auto !important;
  z-index:auto !important;
  min-height:26px !important;
  padding:4px 10px !important;
  border-radius:999px !important;
  border:1px solid #c6d6e8 !important;
  background:#f7fbff !important;
  color:#1f4a78 !important;
  box-shadow:none !important;
  backdrop-filter:none !important;
  font-size:.78rem !important;
  font-weight:760 !important;
}
.wolfbbs-header-command:hover{
  background:#eaf2fb !important;
}
.wolfbbs-help-copy{
  margin:0;
  padding:.75rem .9rem;
  border:1px solid rgba(122,162,205,.26);
  border-left:3px solid rgba(33,109,196,.68);
  border-radius:14px;
  background:linear-gradient(180deg,rgba(235,244,255,.9),rgba(248,251,255,.98));
  color:#3e5b79;
  font-size:.78rem;
  line-height:1.55;
  box-shadow:inset 0 1px 0 rgba(255,255,255,.72);
}
.wolfbbs-pref-menu-panel button{
  display:flex;
  align-items:center;
  width:100%;
  justify-content:flex-start;
  text-align:left;
  border-radius:12px;
}
.wolfbbs-pref-menu-note{
  font-size:.75rem;
}
body.wolfbbs-controls-compact .wolfbbs-pref-controls{
  gap:6px;
}
body.wolfbbs-controls-compact .wolfbbs-pref-menu-note{
  display:none;
}
.wolfbbs-favorites-rail{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  margin:8px 0 10px;
  padding:8px 10px;
  border:1px solid #ccdae9;
  border-radius:11px;
  background:#f9fcff;
}
.wolfbbs-favorites-rail strong{
  color:#3e5c7a;
  font-size:.78rem;
  text-transform:uppercase;
  letter-spacing:.05em;
}
.wolfbbs-favorites-rail a{
  display:inline-flex;
  align-items:center;
  min-height:26px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #c6d7e9;
  background:#ffffff;
  color:#1f4a78;
  font-size:.8rem;
  font-weight:730;
}
.wolfbbs-section-collapsible{
  position:relative;
}
.wolfbbs-section-toggle{
  margin-left:auto;
  min-height:26px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#f7fbff;
  color:#1f4a78;
  box-shadow:none;
  font-size:.78rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:760;
}
.wolfbbs-heading-link{
  display:inline-flex;
  align-items:center;
  margin-left:6px;
  min-height:24px;
  padding:3px 8px;
  border-radius:999px;
  border:1px solid #cad7e8;
  background:#f7fbff;
  color:#315c87;
  font-size:.72rem;
  font-weight:700;
}
.wolfbbs-heading-link:hover{
  background:#edf4fc;
  text-decoration:none;
}
.wolfbbs-section-collapsible.is-collapsed .wolfbbs-section-body{
  display:none;
}
.wolfbbs-table-toolbar{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:8px;
  margin:0 0 8px;
  padding:8px 10px;
  border:1px solid #ccdae9;
  border-radius:11px;
  background:#f8fbff;
}
.wolfbbs-table-toolbar strong{
  color:#365572;
  font-size:.78rem;
  text-transform:uppercase;
  letter-spacing:.05em;
}
.wolfbbs-table-toolbar input[type=search]{
  min-height:28px;
  width:min(280px,100%);
  padding:6px 9px;
  border-radius:9px;
  font-size:.84rem;
}
.wolfbbs-table-toolbar button{
  min-height:28px;
  padding:5px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#ffffff;
  color:#1f4a78;
  box-shadow:none;
  font-size:.78rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:760;
}
.wolfbbs-table-toolbar button:hover{
  background:#edf4fc;
}
.wolfbbs-table-count{
  color:#496884;
  font-size:.8rem;
  font-weight:660;
}
.wolfbbs-table-sort{
  display:inline-flex;
  align-items:center;
  gap:4px;
  min-height:24px;
  padding:0;
  border:0;
  background:none;
  color:inherit;
  box-shadow:none;
  font:inherit;
  text-transform:inherit;
  letter-spacing:inherit;
}
.wolfbbs-table-sort:hover{
  text-decoration:underline;
}
.wolfbbs-table-sort-indicator{
  color:#6783a2;
  font-size:.72rem;
}
.wolfbbs-row-active td{
  background:#e8f2ff !important;
}
.wolfbbs-row-hit td{
  background:#fff8ea;
}
.wolfbbs-empty-row td{
  text-align:center;
  color:#5a728b;
  font-style:italic;
}
.wolfbbs-form-actions-sticky{
  position:sticky;
  bottom:10px;
  z-index:20;
  margin-top:10px;
  padding:8px 10px;
  border:1px solid #cad8e8;
  border-radius:10px;
  background:rgba(255,255,255,.95);
  box-shadow:0 8px 18px rgba(9,41,81,.1);
}
.wolfbbs-form-actions-sticky button{
  min-height:30px;
}
.wolfbbs-invalid{
  border-color:#c44a5c !important;
  box-shadow:0 0 0 3px rgba(185,54,73,.12) !important;
}
.wolfbbs-field-hint{
  display:block;
  margin:4px 0 0;
  color:#7a4d00;
  font-size:.77rem;
  font-weight:650;
}
#wolfbbsBackToTop{
  position:fixed;
  right:18px;
  bottom:66px;
  z-index:39;
  min-height:36px;
  padding:7px 11px;
  border-radius:999px;
  border:1px solid #bed1e8;
  background:#ffffff;
  color:#1f4a78;
  font-size:.8rem;
  font-weight:760;
  box-shadow:0 10px 18px rgba(9,41,81,.14);
  display:none;
}
#wolfbbsBackToTop.show{
  display:inline-flex;
  align-items:center;
}
#wolfbbsToastRegion{
  position:fixed;
  top:14px;
  right:14px;
  z-index:96;
  display:flex;
  flex-direction:column;
  gap:8px;
  pointer-events:none;
}
.wolfbbs-toast{
  min-width:220px;
  max-width:min(360px,80vw);
  padding:8px 11px;
  border-radius:10px;
  border:1px solid #bfd2e8;
  background:#f8fbff;
  color:#1b426d;
  box-shadow:0 10px 20px rgba(9,39,79,.16);
  opacity:0;
  transform:translateY(-4px);
  transition:opacity .16s ease, transform .16s ease;
}
.wolfbbs-toast.show{
  opacity:1;
  transform:translateY(0);
}
.wolfbbs-toast[data-kind="error"]{
  border-color:#e7b8bf;
  background:#fff4f6;
  color:#7f1f2d;
}
.wolfbbs-toast[data-kind="ok"]{
  border-color:#bad9c5;
  background:#f1fbf4;
  color:#1f6242;
}
.wolfbbs-goal-coach{
  display:grid;
  gap:8px;
  margin:8px 0 12px;
  padding:11px 13px;
  border:1px solid #c8d7e8;
  border-left:4px solid #0f5ba8;
  border-radius:12px;
  background:#f9fcff;
}
.wolfbbs-goal-head{
  display:flex;
  flex-wrap:wrap;
  justify-content:space-between;
  gap:8px;
  align-items:center;
}
.wolfbbs-goal-head strong{
  color:#143d65;
}
.wolfbbs-goal-head span{
  color:#45627f;
  font-size:.82rem;
  font-weight:670;
}
.wolfbbs-goal-list{
  list-style:none;
  padding:0;
  margin:0;
  display:grid;
  gap:6px;
}
.wolfbbs-goal-list li{
  display:flex;
  align-items:flex-start;
  gap:8px;
  color:#34526f;
  font-size:.88rem;
}
.wolfbbs-goal-list a{
  margin-left:auto;
  font-size:.8rem;
}
.wolfbbs-goal-actions{
  display:flex;
  gap:8px;
  flex-wrap:wrap;
}
.wolfbbs-goal-actions button{
  min-height:28px;
  padding:5px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#ffffff;
  color:#1f4a78;
  box-shadow:none;
  font-size:.78rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:760;
}
.wolfbbs-scorecard{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:8px;
  margin:8px 0 12px;
  padding:9px 11px;
  border:1px solid #c9d8e8;
  border-radius:11px;
  background:#f8fbff;
}
.wolfbbs-scorecard-note{
  margin:0;
  color:#47627d;
  font-size:.82rem;
}
.wolfbbs-score-pill{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:2px 10px;
  border-radius:999px;
  border:1px solid #c8daf1;
  background:#edf4ff;
  color:#244c83;
  font-size:.79rem;
  font-weight:700;
}
.wolfbbs-score-pill[data-level="strong"]{
  border-color:#bad9c5;
  background:#f1fbf4;
  color:#1f6242;
}
.wolfbbs-score-pill[data-level="warn"]{
  border-color:#f0d7a0;
  background:#fff5df;
  color:#7b5400;
}
.wolfbbs-empty-actions{
  display:flex;
  flex-wrap:wrap;
  gap:7px;
  margin-top:7px;
}
.wolfbbs-empty-actions a{
  display:inline-flex;
  align-items:center;
  min-height:26px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#ffffff;
  color:#1f4a78;
  font-size:.79rem;
  font-weight:740;
}
.wolfbbs-empty-actions a:hover{
  background:#edf4fc;
  text-decoration:none;
}
#wolfbbsNotesButton{
  position:fixed;
  left:20px;
  bottom:18px;
  z-index:40;
  min-height:40px;
  padding:8px 13px;
  border-radius:999px;
  border:1px solid #bed1e8;
  background:#ffffff;
  color:#1f4a78;
  font-size:.82rem;
  font-weight:760;
  box-shadow:0 14px 22px rgba(9,41,81,.15);
}
#wolfbbsNotesOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.52);
  display:none;
  z-index:65;
  padding:22px 16px;
}
#wolfbbsNotesOverlay.active{
  display:block;
}
#wolfbbsNotesPanel{
  max-width:820px;
  margin:0 auto;
  background:linear-gradient(180deg,#ffffff,#f4f9ff);
  border:1px solid var(--line);
  border-radius:18px;
  box-shadow:0 28px 68px rgba(8,19,36,.34);
  overflow:hidden;
}
#wolfbbsNotesHeader{
  padding:12px 14px;
  background:linear-gradient(180deg,#f8fcff,#ebf3ff);
  border-bottom:1px solid var(--line);
  display:flex;
  justify-content:space-between;
  gap:10px;
  align-items:center;
}
#wolfbbsNotesArea{
  width:100%;
  min-height:280px;
  border:0;
  border-top:1px solid #d3dfef;
  border-radius:0;
  padding:12px 13px;
  background:#ffffff;
  resize:vertical;
}
.wolfbbs-notes-actions{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  padding:10px 12px;
}
.wolfbbs-notes-actions button{
  min-height:29px;
  padding:6px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#ffffff;
  color:#1f4a78;
  box-shadow:none;
  font-size:.78rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:760;
}
#wolfbbsMacroHelpOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.45);
  display:none;
  z-index:62;
  padding:22px 16px;
}
#wolfbbsMacroHelpOverlay.active{
  display:block;
}
#wolfbbsMacroHelpPanel{
  max-width:540px;
  margin:0 auto;
  background:#ffffff;
  border:1px solid #c8d7e8;
  border-radius:14px;
  box-shadow:0 20px 40px rgba(8,19,36,.28);
  padding:14px 15px;
}
#wolfbbsMacroHelpPanel ul{
  margin:10px 0 0 18px;
}
#wolfbbsMacroHelpPanel li{
  color:#3a5875;
}
.wolfbbs-template-strip{
  display:flex;
  flex-wrap:wrap;
  gap:7px;
  margin:8px 0 10px;
}
.wolfbbs-template-strip button{
  min-height:26px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#f8fbff;
  color:#1f4a78;
  box-shadow:none;
  font-size:.78rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:760;
}
.wolfbbs-restore-banner{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:8px;
  margin:0 0 10px;
  padding:8px 10px;
  border:1px solid #d8c89b;
  border-radius:10px;
  background:#fff8eb;
  color:#6f4b0b;
  font-size:.83rem;
  font-weight:650;
}
.wolfbbs-focus-pill{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:3px 10px;
  border-radius:999px;
  border:1px solid #c8daf1;
  background:#edf4ff;
  color:#244c83;
  font-size:.77rem;
  font-weight:700;
}
.wolfbbs-focus-pill[data-state="running"]{
  border-color:#bad9c5;
  background:#f1fbf4;
  color:#1f6242;
}
.wolfbbs-live-region{
  position:absolute;
  left:-9999px;
  width:1px;
  height:1px;
  overflow:hidden;
}
body.wolfbbs-focus-ui .wolfbbs-guide-strip,
body.wolfbbs-focus-ui .wolfbbs-recent-rail,
body.wolfbbs-focus-ui .wolfbbs-favorites-rail,
body.wolfbbs-focus-ui .wolfbbs-scorecard{
  display:none !important;
}
.wolfbbs-hero-chip[data-kind="net-online"]{
  background:#edf9f0;
  border-color:#bad9c5;
  color:#1f6242;
}
.wolfbbs-hero-chip[data-kind="net-offline"]{
  background:#fff0f2;
  border-color:#efc1c6;
  color:#8b2331;
}
.wolfbbs-hero-chip[data-kind="latency"]{
  background:#eef4ff;
  border-color:#c5d5ec;
  color:#244f80;
}
.wolfbbs-hero-chip[data-kind="telemetry"]{
  background:#f7f2ff;
  border-color:#d4c5ef;
  color:#4d3781;
}
.wolfbbs-section-progress{
  display:inline-flex;
  align-items:center;
  min-height:26px;
  padding:3px 10px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#f8fbff;
  color:#385877;
  font-size:.79rem;
  font-weight:700;
}
.wolfbbs-column-toggle{
  position:relative;
}
.wolfbbs-column-toggle-panel{
  position:absolute;
  top:34px;
  left:0;
  z-index:26;
  min-width:210px;
  max-height:260px;
  overflow:auto;
  padding:8px 9px;
  border:1px solid #c8d7e8;
  border-radius:10px;
  background:#ffffff;
  box-shadow:0 12px 24px rgba(9,39,79,.16);
  display:none;
}
.wolfbbs-column-toggle-panel.active{
  display:block;
}
.wolfbbs-column-toggle-panel label{
  display:flex;
  flex-direction:row;
  align-items:center;
  gap:8px;
  margin:0 0 7px;
  font-size:.81rem;
  color:#34526f;
}
.wolfbbs-guide-minimized p,
.wolfbbs-guide-minimized .wolfbbs-guide-actions,
.wolfbbs-guide-minimized .wolfbbs-guide-details{
  display:none;
}
.wolfbbs-text-counter{
  display:block;
  margin:4px 0 0;
  color:#57708b;
  font-size:.75rem;
  font-weight:650;
}
#wolfbbsUXDiagButton{
  position:fixed;
  left:140px;
  bottom:18px;
  z-index:40;
  min-height:40px;
  padding:8px 13px;
  border-radius:999px;
  border:1px solid #bed1e8;
  background:#ffffff;
  color:#1f4a78;
  font-size:.82rem;
  font-weight:760;
  box-shadow:0 14px 22px rgba(9,41,81,.15);
}
#wolfbbsShortcutOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.45);
  display:none;
  z-index:63;
  padding:22px 16px;
}
#wolfbbsShortcutOverlay.active{
  display:block;
}
#wolfbbsShortcutPanel{
  max-width:560px;
  margin:0 auto;
  background:#ffffff;
  border:1px solid #c8d7e8;
  border-radius:14px;
  box-shadow:0 20px 40px rgba(8,19,36,.28);
  padding:14px 15px;
}
#wolfbbsShortcutPanel ul{
  margin:10px 0 0 18px;
}
#wolfbbsShortcutPanel li{
  color:#3a5875;
}
.wolfbbs-palette-history{
  display:flex;
  flex-wrap:wrap;
  gap:6px;
  margin-top:8px;
}
.wolfbbs-palette-history button{
  min-height:25px;
  padding:4px 9px;
  border-radius:999px;
  border:1px solid #c6d6e8;
  background:#ffffff;
  color:#315b86;
  box-shadow:none;
  font-size:.76rem;
  font-family:"Manrope","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
  font-weight:700;
}
#wolfbbsToastCenterOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.45);
  display:none;
  z-index:64;
  padding:22px 16px;
}
#wolfbbsToastCenterOverlay.active{
  display:block;
}
#wolfbbsToastCenterPanel{
  max-width:620px;
  margin:0 auto;
  background:#ffffff;
  border:1px solid #c8d7e8;
  border-radius:14px;
  box-shadow:0 20px 40px rgba(8,19,36,.28);
  padding:14px 15px;
}
#wolfbbsToastCenterList{
  max-height:320px;
  overflow:auto;
  margin-top:10px;
}
#wolfbbsToastCenterList li{
  padding:8px 0;
  border-bottom:1px solid #e6eef8;
  color:#365472;
  font-size:.86rem;
}
#wolfbbsToastCenterList li:last-child{
  border-bottom:0;
}
.wolfbbs-revisit-banner{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
  align-items:center;
  margin:8px 0 11px;
  padding:8px 10px;
  border:1px solid #c8d7e8;
  border-radius:10px;
  background:#f8fbff;
  color:#3a5876;
  font-size:.82rem;
  font-weight:650;
}
.wolfbbs-kpi-delta{
  margin-left:6px;
  font-size:.76rem;
  font-weight:700;
  color:#587392;
}
.wolfbbs-kpi-delta.up{
  color:#1f6242;
}
.wolfbbs-kpi-delta.down{
  color:#8b2331;
}
#wolfbbsWorkspaceOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.45);
  display:none;
  z-index:66;
  padding:22px 16px;
}
#wolfbbsWorkspaceOverlay.active{
  display:block;
}
#wolfbbsWorkspacePanel{
  max-width:700px;
  margin:0 auto;
  background:#ffffff;
  border:1px solid #c8d7e8;
  border-radius:14px;
  box-shadow:0 20px 40px rgba(8,19,36,.28);
  padding:14px 15px;
}
#wolfbbsWorkspaceList{
  display:grid;
  gap:8px;
  margin-top:10px;
}
.wolfbbs-workspace-row{
  padding:9px 10px;
  border:1px solid #d4e0ef;
  border-radius:10px;
  background:#fbfdff;
}
.wolfbbs-workspace-row strong{
  display:block;
  color:#163c64;
}
.wolfbbs-workspace-row p{
  margin:4px 0 0;
  color:#4c6883;
  font-size:.82rem;
}
.wolfbbs-workspace-row .wolfbbs-inline-actions{
  margin-top:6px;
}
#wolfbbsSpotlightOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.45);
  display:none;
  z-index:67;
  padding:22px 16px;
}
#wolfbbsSpotlightOverlay.active{
  display:block;
}
#wolfbbsSpotlightPanel{
  max-width:620px;
  margin:0 auto;
  background:#ffffff;
  border:1px solid #c8d7e8;
  border-radius:14px;
  box-shadow:0 20px 40px rgba(8,19,36,.28);
  padding:14px 15px;
}
.wolfbbs-spotlight-hit{
  background:linear-gradient(180deg,#fff6d8,#ffeeb5);
  border-radius:4px;
  padding:0 2px;
}
.wolfbbs-selected-row td{
  background:#eaf5ff !important;
}
.wolfbbs-handoff-box{
  margin:8px 0 12px;
  padding:10px 12px;
  border:1px solid #c9d8e8;
  border-radius:10px;
  background:#f8fbff;
}
.wolfbbs-handoff-box p{
  margin:0;
  color:#3a5876;
  font-size:.83rem;
}
#wolfbbsDraftOverlay,#wolfbbsIncidentOverlay,#wolfbbsPlaybookOverlay,#wolfbbsReminderOverlay,#wolfbbsReleaseGateOverlay,#wolfbbsFeedbackOverlay{
  position:fixed;
  inset:0;
  background:rgba(8,17,30,.45);
  display:none;
  z-index:69;
  padding:22px 16px;
}
#wolfbbsDraftOverlay.active,#wolfbbsIncidentOverlay.active,#wolfbbsPlaybookOverlay.active,#wolfbbsReminderOverlay.active,#wolfbbsReleaseGateOverlay.active,#wolfbbsFeedbackOverlay.active{
  display:block;
}
#wolfbbsDraftPanel,#wolfbbsIncidentPanel,#wolfbbsPlaybookPanel,#wolfbbsReminderPanel,#wolfbbsReleaseGatePanel,#wolfbbsFeedbackPanel{
  max-width:760px;
  margin:0 auto;
  background:#ffffff;
  border:1px solid #c8d7e8;
  border-radius:14px;
  box-shadow:0 20px 40px rgba(8,19,36,.28);
  padding:14px 15px;
}
#wolfbbsDraftList,#wolfbbsIncidentList,#wolfbbsPlaybookList,#wolfbbsReminderList,#wolfbbsReleaseGateList{
  display:grid;
  gap:8px;
  margin-top:10px;
  max-height:360px;
  overflow:auto;
}
.wolfbbs-incident-row,.wolfbbs-playbook-row,.wolfbbs-reminder-row,.wolfbbs-release-gate-row,.wolfbbs-draft-row{
  padding:9px 10px;
  border:1px solid #d4e0ef;
  border-radius:10px;
  background:#fbfdff;
}
.wolfbbs-reminder-row[data-due="true"]{
  border-color:#f0ca85;
  background:#fff8eb;
}
.wolfbbs-incident-badge{
  display:inline-flex;
  min-height:22px;
  align-items:center;
  padding:2px 8px;
  border-radius:999px;
  font-size:.74rem;
  font-weight:720;
}
.wolfbbs-incident-badge[data-severity="high"]{
  background:#fff1f2;
  border:1px solid #efc1c6;
  color:#8b2331;
}
.wolfbbs-incident-badge[data-severity="medium"]{
  background:#fff8eb;
  border:1px solid #ecd39a;
  color:#7d5b10;
}
.wolfbbs-incident-badge[data-severity="low"]{
  background:#edf9f0;
  border:1px solid #bad9c5;
  color:#1f6242;
}
#wolfbbsFeedbackButton{
  position:fixed;
  left:228px;
  bottom:18px;
  z-index:40;
  min-height:40px;
  padding:8px 13px;
  border-radius:999px;
  border:1px solid #bed1e8;
  background:#ffffff;
  color:#1f4a78;
  font-size:.82rem;
  font-weight:760;
  box-shadow:0 14px 22px rgba(9,41,81,.15);
}
.wolfbbs-rating-row{
  display:flex;
  flex-wrap:wrap;
  gap:8px;
}
.wolfbbs-rating-row button[data-active="true"]{
  background:#1f4a78;
  border-color:#1f4a78;
  color:#fff;
}
body[data-layout-mode="wide"]{
  width:min(1480px,calc(100% - 2.2rem));
}
body[data-layout-mode="focus"]{
  width:min(980px,calc(100% - 2rem));
}
body[data-accent-mode="teal"]{
  --accent:#0f9274;
  --accent-strong:#0a6a54;
  --accent-soft:#d6f5ec;
}
body[data-accent-mode="amber"]{
  --accent:#b36a00;
  --accent-strong:#8f4f00;
  --accent-soft:#ffe9c7;
}
body.wolfbbs-motion-reduced *,
body.wolfbbs-motion-reduced *::before,
body.wolfbbs-motion-reduced *::after{
  animation:none !important;
  transition:none !important;
}
#wolfbbsActionDock{
  position:fixed;
  right:18px;
  bottom:18px;
  z-index:45;
  width:min(320px,calc(100vw - 1.6rem));
  background:linear-gradient(180deg,#ffffff,#f2f7ff);
  border:1px solid #c5d7ee;
  border-radius:14px;
  box-shadow:0 16px 26px rgba(9,41,81,.2);
  overflow:hidden;
}
#wolfbbsActionDock[data-collapsed="true"]{
  width:auto;
}
#wolfbbsActionDock[data-side="left"]{
  right:auto;
  left:18px;
}
.wolfbbs-action-dock-head{
  display:flex;
  align-items:center;
  justify-content:space-between;
  gap:8px;
  padding:10px 12px;
  border-bottom:1px solid #d4e3f4;
}
.wolfbbs-action-dock-head strong{
  font-size:.86rem;
  letter-spacing:.02em;
  text-transform:uppercase;
  color:#234d7a;
}
#wolfbbsActionDock[data-collapsed="true"] .wolfbbs-action-dock-head{
  padding:9px 10px;
}
.wolfbbs-action-dock-head button{
  min-height:30px;
  padding:5px 10px;
  font-size:.78rem;
}
.wolfbbs-action-dock-search{
  display:flex;
  gap:6px;
  margin:0 0 4px;
}
.wolfbbs-action-dock-search input[type=search]{
  width:100%;
  min-height:30px;
  padding:5px 9px;
  border-radius:8px;
  border:1px solid #c7d9ef;
  background:#fbfdff;
  color:#1e4a80;
  font-size:.81rem;
}
.wolfbbs-action-dock-body{
  display:grid;
  gap:9px;
  padding:11px 12px 12px;
  max-height:320px;
  overflow:auto;
}
#wolfbbsActionDock[data-collapsed="true"] .wolfbbs-action-dock-body{
  display:none;
}
#wolfbbsActionDock[data-collapsed="true"] .wolfbbs-inline-actions{
  gap:0;
}
.wolfbbs-action-dock-group{
  display:grid;
  gap:6px;
}
.wolfbbs-action-dock-group-title{
  color:#4f6784;
  font-size:.75rem;
  font-weight:760;
  text-transform:uppercase;
  letter-spacing:.05em;
  display:flex;
  justify-content:space-between;
  align-items:center;
}
.wolfbbs-action-dock-group-title span{
  font-size:.72rem;
  color:#6d84a0;
  font-weight:680;
}
.wolfbbs-action-dock-links{
  display:flex;
  flex-wrap:wrap;
  gap:6px;
}
.wolfbbs-action-dock-links a{
  display:inline-flex;
  align-items:center;
  min-height:30px;
  padding:5px 10px;
  border-radius:999px;
  border:1px solid #c6d7ee;
  background:#fff;
  color:#1e4a80;
  font-size:.82rem;
  font-weight:640;
}
.wolfbbs-action-dock-links a:hover{
  text-decoration:none;
  background:#edf6ff;
}
.wolfbbs-session-chip{
  font-variant-numeric:tabular-nums;
}
.wolfbbs-section-pin-rail{
  display:flex;
  flex-wrap:wrap;
  gap:7px;
  margin:0 0 14px;
  padding:8px 10px;
  border:1px dashed #c3d5eb;
  border-radius:12px;
  background:rgba(255,255,255,.7);
}
.wolfbbs-section-pin-rail a{
  display:inline-flex;
  align-items:center;
  min-height:28px;
  padding:4px 10px;
  border-radius:999px;
  border:1px solid #c0d5ee;
  background:#fff;
  color:#1d4d82;
  font-size:.8rem;
  font-weight:650;
}
.wolfbbs-section-pin-button,
.wolfbbs-section-done-toggle{
  margin-left:6px;
  min-height:26px;
  padding:2px 8px;
  border-radius:999px;
  border:1px solid #c4d7ee;
  background:#fff;
  color:#255182;
  font-size:.74rem;
  font-weight:700;
}
.wolfbbs-section-done-toggle[data-done="true"]{
  background:#eaf8ef;
  border-color:#b8dfc4;
  color:#1e6a43;
}
h2.wolfbbs-section-done{
  opacity:.85;
}
.wolfbbs-form-progress{
  display:flex;
  align-items:center;
  gap:8px;
  margin:0 0 10px;
  color:#48607d;
  font-size:.82rem;
}
.wolfbbs-form-progress meter{
  width:180px;
  max-width:100%;
  height:11px;
}
.wolfbbs-table-wrap.wolfbbs-sticky-head table thead th{
  position:sticky;
  top:0;
  z-index:3;
}
.wolfbbs-table-wrap.wolfbbs-table-compact table th,
.wolfbbs-table-wrap.wolfbbs-table-compact table td{
  padding:6px 8px;
}
.wolfbbs-row-inspector{
  margin-top:8px;
  padding:9px 10px;
  border:1px solid #d4e1f0;
  border-radius:10px;
  background:#fbfdff;
  font-size:.84rem;
}
.wolfbbs-row-inspector dl{
  display:grid;
  grid-template-columns:minmax(84px,140px) 1fr;
  gap:4px 8px;
  margin:0;
}
.wolfbbs-row-inspector dt{
  font-weight:700;
  color:#38597b;
}
.wolfbbs-row-inspector dd{
  margin:0;
  color:#233f5f;
}
.wolfbbs-form-id{
  display:block;
  margin-top:8px;
  color:#6d8098;
  font-size:.74rem;
}
#wolfbbsBugButton{
  position:fixed;
  left:342px;
  bottom:18px;
  z-index:40;
  min-height:40px;
  padding:8px 13px;
  border-radius:999px;
  border:1px solid #bed1e8;
  background:#ffffff;
  color:#1f4a78;
  font-size:.82rem;
  font-weight:760;
  box-shadow:0 14px 22px rgba(9,41,81,.15);
}
#wolfbbsNotesButton,
#wolfbbsUXDiagButton,
#wolfbbsFeedbackButton,
#wolfbbsBugButton{
  display:none !important;
}
#wolfbbsBugOverlay{
  position:fixed;
  inset:0;
  z-index:120;
  display:none;
  place-items:center;
  background:rgba(8,16,28,.42);
  padding:12px;
}
#wolfbbsBugOverlay.active{
  display:grid;
}
#wolfbbsBugPanel{
  width:min(760px,100%);
  max-height:92vh;
  overflow:auto;
  background:#fff;
  border-radius:14px;
  border:1px solid #c4d8ef;
  box-shadow:0 22px 36px rgba(9,41,81,.24);
  padding:14px;
}
#wolfbbsBugPanel textarea{
  width:100%;
  min-height:180px;
}
#wolfbbsContextHelpOverlay{
  position:fixed;
  inset:0;
  z-index:119;
  display:none;
  place-items:center;
  background:rgba(8,16,28,.38);
  padding:12px;
}
#wolfbbsContextHelpOverlay.active{
  display:grid;
}
#wolfbbsContextHelpPanel{
  width:min(680px,100%);
  max-height:90vh;
  overflow:auto;
  background:#fff;
  border:1px solid #c5d8ee;
  border-radius:14px;
  box-shadow:0 20px 34px rgba(9,41,81,.22);
  padding:14px;
}
#wolfbbsContextHelpBody{
  margin:0 0 12px;
}
/* Nostalgia pass: preserve classic BBS feel while keeping modern usability. */
:root{
  --bg:#0a1018;
  --bg-alt:#121b27;
  --surface:#142233;
  --surface-2:#1a2a3d;
  --surface-3:#20354d;
  --text:#ebead7;
  --muted:#b8b79d;
  --line:#35506b;
  --line-strong:#4b6f91;
  --accent:#86d4ff;
  --accent-strong:#a4e2ff;
  --accent-soft:#1f364f;
  --teal:#6fdab8;
  --ok:#8ae5a6;
  --warn:#f2cd80;
  --danger:#ff9aa4;
  --shadow-sm:0 8px 16px rgba(0,0,0,.34);
  --shadow:0 14px 30px rgba(0,0,0,.45);
  --shadow-lg:0 24px 52px rgba(0,0,0,.58);
}
body{
  width:min(1160px,calc(100% - 1.8rem));
  padding:18px 0 92px;
  color:var(--text);
  font-family:"IBM Plex Mono","Cascadia Mono","SFMono-Regular","Menlo","Consolas","Liberation Mono",monospace;
  background:
    radial-gradient(960px 360px at 11% -18%, rgba(134,212,255,.14), transparent 66%),
    radial-gradient(800px 280px at 89% -16%, rgba(111,218,184,.11), transparent 68%),
    linear-gradient(180deg,var(--bg),var(--bg-alt));
}
body::before{
  opacity:.34;
  background:
    linear-gradient(rgba(8,13,20,.24), rgba(8,13,20,.24)),
    repeating-linear-gradient(0deg, rgba(158,198,236,.11) 0px, rgba(158,198,236,.11) 1px, transparent 1px, transparent 3px);
}
h1,h2,h3{
  color:#d9f1ff;
  font-family:"Aldrich","IBM Plex Mono","Cascadia Mono","SFMono-Regular","Menlo","Consolas","Liberation Mono",monospace;
  letter-spacing:.045em;
  text-transform:uppercase;
}
body > h1{
  color:#9ee3ff;
  text-shadow:0 0 16px rgba(134,212,255,.35);
}
body > h1:first-of-type::after{
  display:block;
  width:58%;
  height:3px;
  margin-top:8px;
  border-radius:999px;
  background:linear-gradient(90deg, rgba(134,212,255,.94), rgba(111,218,184,.84));
  box-shadow:0 0 18px rgba(134,212,255,.35);
}
a{
  color:#9fe4ff;
}
a:hover{
  color:#c2ecff;
}
body p,
body li{
  color:#cbc9b2;
}
p.wolfbbs-nav-row,body > p:has(> a){
  border-color:#3a5774;
  background:linear-gradient(180deg,rgba(22,35,52,.96),rgba(19,31,47,.95));
  box-shadow:0 10px 22px rgba(0,0,0,.4);
}
p.wolfbbs-nav-row::before,body > p:has(> a)::before{
  background:linear-gradient(90deg, rgba(134,212,255,.08), rgba(111,218,184,.06));
}
p.wolfbbs-nav-row a,body > p:has(> a) a{
  border-color:#47698a;
  background:linear-gradient(180deg,#1c3147,#1a2b3d);
  color:#d6f1ff;
  text-shadow:0 0 10px rgba(134,212,255,.18);
}
p.wolfbbs-nav-row a:hover,body > p:has(> a) a:hover{
  border-color:#628bb3;
  background:linear-gradient(180deg,#24405c,#1f344b);
}
.wolfbbs-nav-more > summary{
  border-color:#47698a;
  background:linear-gradient(180deg,#1c3147,#1a2b3d);
  color:#d6f1ff;
  text-shadow:0 0 10px rgba(134,212,255,.18);
}
.wolfbbs-nav-more[open] > summary{
  border-color:#628bb3;
  background:linear-gradient(180deg,#24405c,#1f344b);
}
.wolfbbs-nav-more-panel{
  border-color:#466888;
  background:linear-gradient(180deg,#162435,#132131);
  box-shadow:0 24px 44px rgba(0,0,0,.42);
}
p.wolfbbs-nav-row a.wolfbbs-nav-active,
body > p:has(> a) a.wolfbbs-nav-active{
  border-color:#7bc5ee;
  background:linear-gradient(180deg,#2d4f6f,#233d55);
  color:#f0f9ff;
  box-shadow:0 0 0 1px rgba(134,212,255,.38), 0 10px 20px rgba(0,0,0,.45);
}
table,
form,
.wolfbbs-card,
.wolfbbs-helper-card,
.wolfbbs-kpi-card,
.wolfbbs-action-card,
.wolfbbs-page-hero,
.wolfbbs-guide-strip,
.wolfbbs-section-nav{
  border-color:#3b5875;
  background:linear-gradient(180deg,rgba(22,35,52,.96),rgba(18,29,43,.96));
  box-shadow:var(--shadow-sm);
}
th{
  background:linear-gradient(180deg,rgba(38,59,82,.95),rgba(32,49,70,.95));
  color:#cdeeff;
}
tr:nth-child(even) td{
  background:rgba(18,29,43,.54);
}
td{
  border-bottom-color:#314a64;
}
input[type=text],input[type=password],input[type=email],input[type=number],input[type=url],input[type=search],select,textarea{
  border-color:#4b6f91;
  background:linear-gradient(180deg,#1a2b3e,#162536);
  color:#f3f3e8;
}
input::placeholder,
textarea::placeholder{
  color:#9fb7cc;
}
input:focus,select:focus,textarea:focus{
  border-color:#8dd8ff;
  box-shadow:0 0 0 3px rgba(134,212,255,.2);
  background:linear-gradient(180deg,#203248,#1a2a3b);
}
button,input[type=submit],input[type=button],
.wolfbbs-compose-toolbar button,
.wolfbbs-handle-assist button,
.wolfbbs-channel-badges button,
.wolfbbs-pref-controls button,
.wolfbbs-pref-menu > summary{
  border:1px solid #5f88ac;
  color:#eef9ff;
  font-family:inherit;
  background:linear-gradient(180deg,#365876,#263f56);
  box-shadow:0 6px 14px rgba(0,0,0,.35);
}
button:hover,input[type=submit]:hover,input[type=button]:hover,
.wolfbbs-compose-toolbar button:hover,
.wolfbbs-handle-assist button:hover,
.wolfbbs-channel-badges button:hover,
.wolfbbs-pref-controls button:hover,
.wolfbbs-pref-menu > summary:hover{
  background:linear-gradient(180deg,#3f6689,#2c4862);
  border-color:#81abd2;
}
.wolfbbs-chip,
.wolfbbs-form-status,
.wolfbbs-status-pill,
.wolfbbs-chat-status-pill,
.wolfbbs-hero-chip,
.wolfbbs-breadcrumbs a,
.wolfbbs-chart-legend span{
  border-color:#4f7394;
  background:linear-gradient(180deg,#1f354c,#1a2d40);
  color:#d7f2ff;
}
.wolfbbs-form-status.dirty,
.wolfbbs-status-pill.warn,
.wolfbbs-chat-status-pill[data-state="warn"],
.wolfbbs-hero-chip[data-kind="guest"]{
  border-color:#8b6b35;
  background:linear-gradient(180deg,#4f3d21,#3f311c);
  color:#f4dca6;
}
.wolfbbs-form-status.error,
.wolfbbs-status-pill.danger,
.wolfbbs-chat-status-pill[data-state="error"]{
  border-color:#8f5160;
  background:linear-gradient(180deg,#4a2430,#3a1d28);
  color:#ffc9d0;
}
.wolfbbs-status-pill.ok,
.wolfbbs-chat-status-pill[data-state="live"],
.wolfbbs-hero-chip[data-kind="admin"]{
  border-color:#4e8b72;
  background:linear-gradient(180deg,#1f4134,#19352b);
  color:#b7f4d9;
}
.wolfbbs-kpi-card strong{
  color:#9fe5ff;
  background:none;
  -webkit-text-fill-color:currentColor;
  text-shadow:0 0 14px rgba(134,212,255,.22);
}
.wolfbbs-card:hover,
.wolfbbs-helper-card:hover,
.wolfbbs-action-card:hover,
.wolfbbs-kpi-card:hover{
  box-shadow:var(--shadow);
}
.wolfbbs-chat-pane{
  border-color:#4f779b;
  border-top-color:#86d4ff;
  background:
    radial-gradient(circle at 84% -12%, rgba(134,212,255,.2), transparent 46%),
    radial-gradient(circle at 16% 112%, rgba(111,218,184,.14), transparent 44%),
    linear-gradient(180deg,#0b141f,#09111b);
}
.wolfbbs-chat-line{
  border-bottom-color:rgba(156,190,224,.15);
}
.wolfbbs-chat-line-self{
  border-left-color:#86d4ff;
  background:rgba(58,98,136,.3);
}
.wolfbbs-chat-line-system{
  border-left-color:#6fdab8;
  background:rgba(42,85,69,.32);
}
.wolfbbs-chat-line-mention{
  border-left-color:#f2cd80;
  background:rgba(86,68,38,.33);
}
#chatStatus{
  color:#9edfff !important;
}
#wolfbbsPalette,
#wolfbbsContextHelpPanel{
  border-color:#466888;
  background:linear-gradient(180deg,#162435,#132131);
}
#wolfbbsPaletteHeader{
  border-bottom-color:#3b5773;
  background:linear-gradient(180deg,#203449,#1a2a3d);
}
*::-webkit-scrollbar-track{
  background:rgba(30,49,68,.5);
}
*::-webkit-scrollbar-thumb{
  border-color:rgba(16,24,34,.85);
  background:linear-gradient(180deg,#5f87ae,#446689);
}
*::-webkit-scrollbar-thumb:hover{
  background:linear-gradient(180deg,#6e98c4,#4f759a);
}
body[data-theme-mode="contrast"]{
  --text:#f8f8ee;
  --muted:#dad6bb;
  --line:#5d7898;
}
body[data-theme-mode="contrast"] a{
  color:#d7efff;
}
body[data-theme-mode="night"]{
  --bg:#070d15;
  --bg-alt:#101824;
  --surface:#122033;
  --surface-2:#17273a;
  --surface-3:#1b3047;
}
.wolfbbs-ux20-compass{
  display:grid;
  grid-template-columns:minmax(0,1fr);
  gap:8px;
  align-items:start;
  margin:10px 0 12px;
  padding:10px 12px;
  border:1px solid #456888;
  border-radius:12px;
  background:linear-gradient(180deg,rgba(24,40,58,.95),rgba(18,30,43,.95));
  box-shadow:0 10px 20px rgba(0,0,0,.32);
}
.wolfbbs-ux20-compass strong{
  display:block;
  color:#d8f0ff;
  margin-bottom:4px;
}
.wolfbbs-ux20-compass p{
  margin:0;
  color:#bfc9b4;
}
.wolfbbs-ux20-path{
  color:#9edfff;
  font-size:.79rem;
}
.wolfbbs-ux20-action-row{
  display:flex;
  flex-wrap:wrap;
  gap:7px;
  margin-top:8px;
}
.wolfbbs-ux20-action-row a{
  display:inline-flex;
  align-items:center;
  min-height:28px;
  padding:4px 11px;
  border-radius:999px;
  border:1px solid #587da1;
  background:linear-gradient(180deg,#29455f,#21364b);
  color:#dbf3ff;
  font-size:.82rem;
  font-weight:700;
}
.wolfbbs-ux20-action-row a:hover{
  text-decoration:none;
  background:linear-gradient(180deg,#335471,#294359);
}
.wolfbbs-ux20-metrics-summary{
  margin:8px 0 0;
  color:#9eb8d2;
  font-size:.8rem;
}
#wolfbbsUX20PrimaryAction{
  position:fixed;
  right:164px;
  bottom:18px;
  z-index:43;
  min-height:40px;
  padding:8px 13px;
  border-radius:999px;
  border:1px solid #86ccef;
  background:linear-gradient(180deg,#335878,#29455f);
  color:#f2fbff;
  box-shadow:0 12px 24px rgba(0,0,0,.38);
}
#wolfbbsUX20PrimaryAction:hover{
  background:linear-gradient(180deg,#40698c,#325471);
}
.wolfbbs-ux20-next{
  margin:10px 0 0;
  padding:12px 14px;
  border:1px solid #3f607f;
  border-radius:12px;
  background:linear-gradient(180deg,rgba(25,39,56,.95),rgba(18,30,43,.95));
}
.wolfbbs-ux20-next strong{
  display:block;
  margin-bottom:7px;
  color:#d8f0ff;
}
.wolfbbs-ux20-next ul{
  margin:0;
  padding-left:18px;
}
.wolfbbs-ux20-next li{
  color:#c8ccb0;
}
.wolfbbs-ux20-form-meter{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:8px;
  margin:0 0 10px;
  padding:8px 10px;
  border:1px solid #466888;
  border-radius:10px;
  background:linear-gradient(180deg,#1f3349,#1b2d40);
}
.wolfbbs-ux20-form-meter span{
  color:#d3ecff;
  font-size:.8rem;
  font-weight:700;
}
.wolfbbs-ux20-meter{
  flex:1 1 160px;
  min-width:160px;
  height:8px;
  border-radius:999px;
  border:1px solid #4b7093;
  background:#152638;
  overflow:hidden;
}
.wolfbbs-ux20-meter b{
  display:block;
  width:0%;
  height:100%;
  background:linear-gradient(90deg,#77cbf3,#7ae0bd);
}
.wolfbbs-ux20-required{
  margin-left:4px;
  color:#f2cd80;
  font-weight:800;
}
.wolfbbs-ux20-search-wrap{
  display:inline-flex;
  align-items:center;
  gap:6px;
}
.wolfbbs-ux20-search-wrap input[type=search]{
  margin:0;
}
.wolfbbs-ux20-clear{
  min-height:28px;
  padding:4px 9px;
  border-radius:999px;
  border:1px solid #5e84a8;
  background:linear-gradient(180deg,#2b4964,#233b52);
  color:#def4ff;
  box-shadow:none;
}
.wolfbbs-ux20-clear:hover{
  background:linear-gradient(180deg,#355979,#2a465f);
}
.wolfbbs-ux20-section-filter{
  min-width:200px;
}
.wolfbbs-ux20-section-meter{
  display:inline-flex;
  align-items:center;
  min-height:24px;
  padding:2px 9px;
  border-radius:999px;
  border:1px solid #567a9b;
  background:linear-gradient(180deg,#1f354c,#1a2d40);
  color:#d7f2ff;
  font-size:.76rem;
  font-weight:700;
}
.wolfbbs-ux20-toolbar-button{
  min-height:28px;
  padding:4px 9px;
  border-radius:999px;
  border:1px solid #5f88ac;
  background:linear-gradient(180deg,#335674,#28445d);
  color:#eaf7ff;
  box-shadow:none;
  font-size:.8rem;
}
table.wolfbbs-ux20-freeze-col th:first-child,
table.wolfbbs-ux20-freeze-col td:first-child{
  position:sticky;
  left:0;
  z-index:3;
  background:linear-gradient(180deg,#243c55,#1c3044);
}
table.wolfbbs-ux20-freeze-col td:first-child{
  z-index:2;
}
.wolfbbs-ux20-highlight{
  background:rgba(87,141,194,.22);
}
.wolfbbs-hero-chip[data-kind="ux20"]{
  border-color:#5583a7;
  background:linear-gradient(180deg,#21384f,#1a2e42);
  color:#d7f2ff;
}
@media (max-width: 820px){
  .wolfbbs-ux20-compass{
    grid-template-columns:1fr;
  }
  .wolfbbs-ux20-metrics{
    justify-content:flex-start;
  }
  #wolfbbsUX20PrimaryAction{
    right:10px;
    left:10px;
    bottom:58px;
  }
  .wolfbbs-pref-controls{
    margin-left:0;
  }
  .wolfbbs-breadcrumbs{
    margin-bottom:6px;
  }
  #wolfbbsBackToTop{
    right:10px;
    bottom:60px;
  }
  #wolfbbsNotesButton{
    left:10px;
    bottom:60px;
  }
  #wolfbbsUXDiagButton{
    left:124px;
    bottom:60px;
  }
  #wolfbbsFeedbackButton{
    left:10px;
    bottom:108px;
  }
  #wolfbbsBugButton{
    left:124px;
    bottom:108px;
  }
  #wolfbbsActionDock{
    left:10px;
    right:10px;
    width:auto;
    bottom:154px;
  }
  #wolfbbsActionDock[data-side="left"],
  #wolfbbsActionDock[data-side="right"]{
    left:10px;
    right:10px;
  }
  p.wolfbbs-nav-row,body > p:has(> a){
    position:static;
  }
  .wolfbbs-page-hero{
    flex-direction:column;
    gap:9px;
  }
  .wolfbbs-page-hero-meta{
    justify-content:flex-start;
  }
  .wolfbbs-section-nav{
    position:static;
  }
  .wolfbbs-guide-strip{
    grid-template-columns:1fr;
  }
  .wolfbbs-guide-actions{
    justify-content:flex-start;
  }
  .wolfbbs-dashboard{
    grid-template-columns:1fr;
  }
  .wolfbbs-chart-shell{
    height:152px;
  }
  .wolfbbs-dash-mini{
    grid-template-columns:1fr 1fr;
  }
  body > h1:first-of-type::after{
    width:56%;
  }
}
@media (prefers-reduced-motion:no-preference){
  .wolfbbs-card,
  .wolfbbs-kpi-card,
  .wolfbbs-action-card,
  .wolfbbs-helper-card,
  .wolfbbs-banner{
    animation:wolfbbsRise .34s ease both;
  }
}
@keyframes wolfbbsRise{
  from{
    opacity:0;
    transform:translateY(6px);
  }
  to{
    opacity:1;
    transform:translateY(0);
  }
}
:root{
  --bg:#080a14;
  --bg-alt:#111525;
  --surface:rgba(12,16,27,.72);
  --surface-2:rgba(18,24,38,.8);
  --surface-3:rgba(28,36,56,.84);
  --text:#f4f7ff;
  --muted:#9aa8c9;
  --line:rgba(150,172,235,.17);
  --line-strong:rgba(173,222,255,.34);
  --accent:#49b6ff;
  --accent-strong:#8cdbff;
  --accent-soft:rgba(73,182,255,.16);
  --accent-2:#ff53d5;
  --accent-3:#7cffd1;
  --shadow-sm:0 18px 42px rgba(2,5,14,.24);
  --shadow:0 28px 78px rgba(1,4,14,.34);
  --shadow-lg:0 44px 120px rgba(0,0,0,.46);
  --radius:2.5rem;
  --radius-sm:1.4rem;
}
html{
  scroll-behavior:smooth;
}
body{
  width:min(1440px,calc(100% - 2.4rem));
  padding:24px 0 96px;
  color:var(--text);
  font-family:"Manrope","IBM Plex Sans","Segoe UI","Helvetica Neue",sans-serif;
  background:
    radial-gradient(1200px 680px at 12% -4%, rgba(73,182,255,.18) 0%, transparent 54%),
    radial-gradient(960px 560px at 88% 0%, rgba(255,83,213,.18) 0%, transparent 52%),
    radial-gradient(860px 500px at 56% -18%, rgba(124,255,209,.14) 0%, transparent 56%),
    linear-gradient(180deg,#05070d 0%,#090d16 22%,#0b1120 52%,#09111f 100%);
}
body::before{
  opacity:.92;
  background:
    radial-gradient(circle at 14% 18%, rgba(73,182,255,.12), transparent 26%),
    radial-gradient(circle at 84% 12%, rgba(255,83,213,.1), transparent 22%),
    linear-gradient(rgba(255,255,255,.045), rgba(255,255,255,.015)),
    repeating-linear-gradient(90deg, rgba(110,160,255,.045) 0px, rgba(110,160,255,.045) 1px, transparent 1px, transparent 28px),
    repeating-linear-gradient(0deg, rgba(255,255,255,.02) 0px, rgba(255,255,255,.02) 1px, transparent 1px, transparent 28px);
}
body::after{
  content:"";
  position:fixed;
  inset:auto 6% 8% auto;
  width:min(42vw,520px);
  aspect-ratio:1;
  pointer-events:none;
  z-index:-1;
  opacity:.55;
  filter:blur(42px);
  background:
    radial-gradient(circle at 34% 36%, rgba(73,182,255,.46) 0%, transparent 38%),
    radial-gradient(circle at 72% 42%, rgba(255,83,213,.34) 0%, transparent 32%),
    radial-gradient(circle at 48% 72%, rgba(124,255,209,.24) 0%, transparent 34%);
}
body[data-theme-mode="default"],
body[data-theme-mode="night"]{
  color:var(--text);
}
body[data-theme-mode="contrast"]{
  --surface:rgba(5,7,13,.86);
  --surface-2:rgba(12,16,24,.92);
  --surface-3:rgba(20,24,37,.95);
  --line:rgba(215,230,255,.3);
  --line-strong:rgba(255,255,255,.5);
  --text:#ffffff;
  --muted:#c1d0ef;
}
body[data-accent-mode="teal"]{
  --accent:#59e4c5;
  --accent-strong:#b4fff1;
  --accent-soft:rgba(89,228,197,.18);
  --accent-2:#50ffe3;
}
body[data-accent-mode="amber"]{
  --accent:#ffc66d;
  --accent-strong:#ffe4a4;
  --accent-soft:rgba(255,198,109,.16);
  --accent-2:#ff8d57;
}
::selection{
  background:rgba(73,182,255,.26);
  color:#fff;
}
h1,h2,h3{
  color:var(--text);
  font-family:"Sora","Space Grotesk","Avenir Next","Segoe UI","Helvetica Neue",sans-serif;
}
body > h1,
.wolfbbs-page-hero h1{
  background:linear-gradient(135deg,#dff2ff 0%,#9bd8ff 34%,#7cffd1 62%,#ff8ce8 100%);
  -webkit-background-clip:text;
  background-clip:text;
  -webkit-text-fill-color:transparent;
  color:transparent;
  text-shadow:none;
}
body p,
body li,
body td,
body th,
body label,
body small{
  color:rgba(233,240,255,.82);
}
a{
  color:var(--accent-strong);
}
a:hover{
  color:#ffffff;
}
button,
input[type=text],
input[type=password],
input[type=email],
input[type=number],
input[type=url],
input[type=search],
select,
textarea{
  border-radius:1.2rem;
  border:1px solid rgba(180,203,255,.18);
  background:linear-gradient(180deg,rgba(18,24,38,.88),rgba(9,13,24,.82));
  color:var(--text);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.08), 0 14px 34px rgba(0,0,0,.24);
}
button:hover,
input[type=text]:hover,
input[type=password]:hover,
input[type=email]:hover,
input[type=number]:hover,
input[type=url]:hover,
input[type=search]:hover,
select:hover,
textarea:hover{
  border-color:rgba(188,224,255,.28);
}
button:focus,
input[type=text]:focus,
input[type=password]:focus,
input[type=email]:focus,
input[type=number]:focus,
input[type=url]:focus,
input[type=search]:focus,
select:focus,
textarea:focus{
  outline:none;
  border-color:rgba(120,211,255,.62);
  box-shadow:0 0 0 1px rgba(120,211,255,.4), 0 18px 44px rgba(15,45,92,.34), inset 0 1px 0 rgba(255,255,255,.12);
}
.wolfbbs-page-hero,
.wolfbbs-card,
.wolfbbs-helper-card,
.wolfbbs-kpi-card,
.wolfbbs-action-card,
.wolfbbs-banner,
.wolfbbs-primer,
.wolfbbs-breadcrumbs,
.wolfbbs-revisit-banner,
form,
#wolfbbsPalette,
.wolfbbs-spatial-card{
  position:relative;
  overflow:hidden;
  border-radius:var(--radius);
  border:1px solid rgba(185,210,255,.15);
  background:
    linear-gradient(180deg,rgba(27,35,54,.58),rgba(10,13,24,.88)),
    linear-gradient(135deg,rgba(255,255,255,.05),rgba(255,255,255,0));
  box-shadow:
    inset 0 1px 0 rgba(255,255,255,.11),
    inset 0 -18px 34px rgba(4,8,16,.24),
    0 28px 78px rgba(1,4,14,.34);
  backdrop-filter:blur(30px) saturate(160%);
  -webkit-backdrop-filter:blur(30px) saturate(160%);
}
.wolfbbs-page-hero::before,
.wolfbbs-card::before,
.wolfbbs-helper-card::before,
.wolfbbs-kpi-card::before,
.wolfbbs-action-card::before,
.wolfbbs-banner::before,
.wolfbbs-primer::before,
.wolfbbs-breadcrumbs::before,
.wolfbbs-revisit-banner::before,
form::before,
#wolfbbsPalette::before,
.wolfbbs-spatial-card::before{
  content:"";
  position:absolute;
  inset:0;
  padding:1px;
  border-radius:inherit;
  pointer-events:none;
  background:linear-gradient(135deg,rgba(255,255,255,.36),rgba(73,182,255,.16) 36%,rgba(255,83,213,.22) 68%,rgba(255,255,255,.04));
  -webkit-mask:
    linear-gradient(#fff 0 0) content-box,
    linear-gradient(#fff 0 0);
  -webkit-mask-composite:xor;
  mask-composite:exclude;
}
table,
.wolfbbs-table-wrap{
  border-radius:calc(var(--radius) - .5rem);
  border:1px solid rgba(185,210,255,.12);
  background:linear-gradient(180deg,rgba(20,28,44,.72),rgba(9,13,23,.9));
  box-shadow:0 20px 52px rgba(0,0,0,.26);
}
thead th{
  background:rgba(255,255,255,.03);
}
tbody tr:nth-child(even){
  background:rgba(255,255,255,.02);
}
tbody tr:hover{
  background:rgba(73,182,255,.08);
}
p.wolfbbs-nav-row,
body > p:has(> a){
  border-radius:999px;
  border:1px solid rgba(180,203,255,.16);
  background:linear-gradient(180deg,rgba(17,23,38,.82),rgba(9,13,24,.84));
  box-shadow:0 22px 54px rgba(0,0,0,.22);
  backdrop-filter:blur(24px);
}
p.wolfbbs-nav-row a,
body > p:has(> a) a{
  color:rgba(239,245,255,.88);
}
.wolfbbs-page-hero{
  grid-column:1 / -1;
  display:grid;
  grid-template-columns:minmax(0,1.6fr) minmax(260px,.95fr);
  gap:1.35rem;
  align-items:start;
  padding:2rem;
  min-height:220px;
  background:
    radial-gradient(circle at 12% 8%, rgba(73,182,255,.18) 0%, transparent 34%),
    radial-gradient(circle at 86% 16%, rgba(255,83,213,.14) 0%, transparent 32%),
    linear-gradient(180deg,rgba(28,36,56,.72),rgba(9,13,24,.9));
}
.wolfbbs-page-hero-main{
  display:grid;
  gap:.8rem;
  min-width:0;
}
.wolfbbs-page-hero-main p{
  max-width:62ch;
  font-size:1rem;
  color:rgba(228,236,255,.76);
}
.wolfbbs-page-hero-meta{
  display:flex;
  flex-wrap:wrap;
  justify-content:flex-end;
  align-content:start;
  gap:.7rem;
}
.wolfbbs-hero-chip{
  min-height:36px;
  padding:.5rem .95rem;
  border-radius:999px;
  border:1px solid rgba(195,220,255,.17);
  background:linear-gradient(180deg,rgba(255,255,255,.08),rgba(255,255,255,.02));
  color:var(--text);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.1), 0 12px 28px rgba(0,0,0,.18);
}
.wolfbbs-hero-chip[data-kind="admin"]{
  color:#ffd6f6;
}
.wolfbbs-hero-chip[data-kind="caller"]{
  color:#cdeaff;
}
.wolfbbs-hero-chip[data-kind="guest"]{
  color:#d0ffe8;
}
.wolfbbs-main{
  display:grid;
  grid-template-columns:repeat(12,minmax(0,1fr));
  gap:1.25rem;
  align-items:start;
}
.wolfbbs-main > *{
  grid-column:1 / -1;
  min-width:0;
}
.wolfbbs-grid,
.wolfbbs-card-grid,
.wolfbbs-kpi-grid,
.wolfbbs-action-grid,
.wolfbbs-helper-grid{
  display:grid;
  grid-template-columns:repeat(12,minmax(0,1fr));
  grid-auto-flow:dense;
  gap:1rem;
  margin:0;
  container-type:inline-size;
}
.wolfbbs-grid > *,
.wolfbbs-card-grid > *,
.wolfbbs-action-grid > *,
.wolfbbs-helper-grid > *{
  grid-column:span 4;
  min-width:0;
  container-type:inline-size;
}
.wolfbbs-kpi-grid > *{
  grid-column:span 3;
  min-width:0;
  container-type:inline-size;
}
.wolfbbs-grid > [data-density="high"],
.wolfbbs-card-grid > [data-density="high"],
.wolfbbs-helper-grid > [data-density="high"]{
  grid-column:span 8;
}
.wolfbbs-kpi-grid > [data-density="high"],
.wolfbbs-action-grid > [data-density="high"]{
  grid-column:span 6;
}
.wolfbbs-grid > [data-density="low"],
.wolfbbs-card-grid > [data-density="low"],
.wolfbbs-helper-grid > [data-density="low"]{
  grid-column:span 3;
}
.wolfbbs-grid > .wolfbbs-primer,
.wolfbbs-grid > .wolfbbs-spatial-card,
.wolfbbs-main > .wolfbbs-spatial-card,
.wolfbbs-main > .wolfbbs-chat-layout,
.wolfbbs-main > .wolfbbs-dashboard,
.wolfbbs-main > .wolfbbs-table-wrap,
.wolfbbs-main > form{
  grid-column:1 / -1;
}
.wolfbbs-card,
.wolfbbs-helper-card,
.wolfbbs-kpi-card,
.wolfbbs-action-card,
.wolfbbs-primer,
.wolfbbs-banner{
  padding:1.35rem 1.45rem;
}
.wolfbbs-kpi-card{
  min-height:160px;
  justify-content:flex-end;
  background:
    radial-gradient(circle at 112% -18%, rgba(73,182,255,.28) 0%, transparent 48%),
    radial-gradient(circle at 0% 100%, rgba(255,83,213,.12) 0%, transparent 36%),
    linear-gradient(180deg,rgba(24,32,49,.72),rgba(9,13,24,.92));
}
.wolfbbs-kpi-card strong{
  font-size:clamp(2rem,5cqi,3rem);
}
.wolfbbs-kpi-card span{
  color:rgba(227,234,255,.72);
}
.wolfbbs-card{
  background:
    radial-gradient(circle at 100% -10%, rgba(73,182,255,.18) 0%, transparent 42%),
    linear-gradient(180deg,rgba(21,28,44,.74),rgba(10,14,24,.9));
}
.wolfbbs-helper-card{
  background:
    radial-gradient(circle at 92% 0%, rgba(124,255,209,.16) 0%, transparent 38%),
    linear-gradient(180deg,rgba(20,28,43,.74),rgba(8,12,22,.9));
}
.wolfbbs-helper-card strong,
.wolfbbs-action-card strong,
.wolfbbs-card strong{
  color:#f3f7ff;
}
.wolfbbs-action-card{
  min-height:164px;
  justify-content:space-between;
  background:
    radial-gradient(circle at 100% -6%, rgba(255,83,213,.16) 0%, transparent 36%),
    radial-gradient(circle at 0% 100%, rgba(73,182,255,.14) 0%, transparent 36%),
    linear-gradient(180deg,rgba(22,29,46,.76),rgba(8,12,22,.92));
}
.wolfbbs-action-card span,
.wolfbbs-helper-card p,
.wolfbbs-card p{
  color:rgba(229,236,255,.72);
}
.wolfbbs-action-card:hover,
.wolfbbs-card:hover,
.wolfbbs-helper-card:hover,
.wolfbbs-kpi-card:hover{
  transform:translateY(-4px);
  border-color:rgba(205,228,255,.24);
  box-shadow:0 34px 92px rgba(0,0,0,.38);
}
.wolfbbs-chip,
.wolfbbs-section-nav a,
.wolfbbs-recent-rail a,
.wolfbbs-primer-actions a{
  border:1px solid rgba(192,216,255,.16);
  background:linear-gradient(180deg,rgba(255,255,255,.08),rgba(255,255,255,.03));
  color:rgba(241,246,255,.88);
}
.wolfbbs-banner[data-kind="notice"]{
  border-color:rgba(124,255,209,.24);
}
.wolfbbs-banner[data-kind="error"]{
  border-color:rgba(255,123,150,.28);
}
.wolfbbs-banner[data-kind="warn"]{
  border-color:rgba(255,198,109,.32);
}
main.wolfbbs-main h2,
main.wolfbbs-main h3{
  display:flex;
  flex-wrap:wrap;
  align-items:center;
  gap:.45rem .55rem;
}
main.wolfbbs-main h2 > .wolfbbs-heading-link,
main.wolfbbs-main h2 > .wolfbbs-section-pin-button,
main.wolfbbs-main h2 > .wolfbbs-section-done-toggle,
main.wolfbbs-main h3 > .wolfbbs-heading-link,
main.wolfbbs-main h3 > .wolfbbs-section-pin-button,
main.wolfbbs-main h3 > .wolfbbs-section-done-toggle{
  margin-left:0;
}
.wolfbbs-section-toggle,
.wolfbbs-heading-link,
.wolfbbs-table-toolbar button,
.wolfbbs-notes-actions button,
.wolfbbs-template-strip button,
.wolfbbs-section-progress,
.wolfbbs-section-pin-button,
.wolfbbs-section-done-toggle,
.wolfbbs-goal-actions button,
.wolfbbs-empty-actions a,
.wolfbbs-action-dock-links a,
.wolfbbs-palette-history button,
.wolfbbs-pref-menu > summary,
#wolfbbsNotesButton,
#wolfbbsUXDiagButton,
#wolfbbsFeedbackButton,
#wolfbbsBackToTop{
  border:1px solid rgba(195,220,255,.16);
  background:linear-gradient(180deg,rgba(255,255,255,.11),rgba(255,255,255,.03)) !important;
  color:rgba(244,247,255,.9) !important;
  box-shadow:inset 0 1px 0 rgba(255,255,255,.09), 0 14px 26px rgba(0,0,0,.18);
}
.wolfbbs-table-toolbar,
.wolfbbs-form-actions-sticky,
.wolfbbs-goal-coach,
.wolfbbs-scorecard,
.wolfbbs-section-pin-rail,
.wolfbbs-handoff-box,
.wolfbbs-column-toggle-panel,
#wolfbbsActionDock,
.wolfbbs-pref-menu-panel,
#wolfbbsNotesPanel,
#wolfbbsMacroHelpPanel,
#wolfbbsShortcutPanel,
#wolfbbsToastCenterPanel,
#wolfbbsWorkspacePanel,
#wolfbbsSpotlightPanel,
#wolfbbsDraftPanel,
#wolfbbsIncidentPanel,
#wolfbbsPlaybookPanel,
#wolfbbsReminderPanel,
#wolfbbsReleaseGatePanel,
#wolfbbsFeedbackPanel,
#wolfbbsContextHelpPanel{
  border:1px solid rgba(185,210,255,.14) !important;
  background:
    linear-gradient(180deg,rgba(24,31,48,.82),rgba(8,12,22,.92)) !important;
  color:var(--text);
  box-shadow:
    inset 0 1px 0 rgba(255,255,255,.09),
    0 22px 54px rgba(0,0,0,.28) !important;
  backdrop-filter:blur(24px);
  -webkit-backdrop-filter:blur(24px);
}
#wolfbbsNotesArea,
.wolfbbs-action-dock-search input[type=search],
.wolfbbs-table-toolbar input[type=search]{
  background:linear-gradient(180deg,rgba(17,23,37,.94),rgba(10,14,24,.88)) !important;
  color:var(--text) !important;
  border:1px solid rgba(180,203,255,.16) !important;
}
.wolfbbs-section-progress,
.wolfbbs-table-count,
.wolfbbs-kpi-delta,
.wolfbbs-text-counter,
.wolfbbs-workspace-row p,
.wolfbbs-handoff-box p,
.wolfbbs-toast,
#wolfbbsToastCenterList li,
#wolfbbsShortcutPanel li,
#wolfbbsMacroHelpPanel li{
  color:rgba(224,233,255,.7) !important;
}
.wolfbbs-section-pin-rail{
  border-style:solid;
}
.wolfbbs-section-pin-rail .wolfbbs-muted{
  color:rgba(224,233,255,.62) !important;
}
#wolfbbsActionDock{
  bottom:92px;
  border-radius:1.3rem;
}
.wolfbbs-action-dock-head,
#wolfbbsNotesHeader{
  border-bottom:1px solid rgba(185,210,255,.12) !important;
}
.wolfbbs-action-dock-head strong,
.wolfbbs-action-dock-group-title,
.wolfbbs-workspace-row strong{
  color:rgba(245,248,255,.92) !important;
}
.wolfbbs-action-dock-group-title span{
  color:rgba(224,233,255,.62) !important;
}
.wolfbbs-pref-menu[open] > summary{
  background:linear-gradient(180deg,rgba(58,84,122,.94),rgba(31,46,73,.96)) !important;
}
.wolfbbs-help-copy{
  border-color:rgba(162,206,255,.18) !important;
  border-left-color:rgba(108,188,255,.72) !important;
  background:linear-gradient(180deg,rgba(24,35,54,.9),rgba(12,18,30,.96)) !important;
  color:rgba(224,233,255,.78) !important;
  box-shadow:inset 0 1px 0 rgba(255,255,255,.05), 0 12px 26px rgba(0,0,0,.16);
}
.wolfbbs-pref-menu-note{
  color:inherit !important;
}
.wolfbbs-toast{
  background:linear-gradient(180deg,rgba(17,24,38,.94),rgba(8,12,22,.94)) !important;
}
.wolfbbs-toast[data-kind="ok"]{
  border-color:rgba(124,255,209,.22) !important;
}
.wolfbbs-toast[data-kind="error"]{
  border-color:rgba(255,123,150,.24) !important;
}
.wolfbbs-form-actions-sticky{
  bottom:18px;
}
#wolfbbsNotesButton,
#wolfbbsUXDiagButton,
#wolfbbsFeedbackButton{
  bottom:20px;
}
#wolfbbsNotesButton{left:18px}
#wolfbbsUXDiagButton{left:136px}
#wolfbbsFeedbackButton{left:246px}
#chat{
  min-height:360px;
  border:1px solid rgba(195,220,255,.16) !important;
  background:
    radial-gradient(circle at 20% 0%, rgba(73,182,255,.12), transparent 38%),
    linear-gradient(180deg,rgba(12,17,29,.96),rgba(6,9,16,.98));
}
#chatStatus{
  color:#d0e9ff !important;
}
#mod form{
  background:linear-gradient(180deg,rgba(22,30,47,.78),rgba(8,12,22,.9));
}
#wolfbbsCommandButton:not(.wolfbbs-header-command){
  left:auto;
  right:24px;
  bottom:20px;
  z-index:55;
  min-height:54px;
  padding:.9rem 1.2rem;
  gap:.55rem;
  border-radius:999px;
  border:1px solid rgba(195,220,255,.2);
  background:
    linear-gradient(135deg,rgba(73,182,255,.92),rgba(31,100,255,.84) 52%,rgba(255,83,213,.88));
  color:#ffffff;
  box-shadow:0 30px 72px rgba(7,18,40,.5), inset 0 1px 0 rgba(255,255,255,.24);
  backdrop-filter:blur(18px);
}
#wolfbbsCommandButton.wolfbbs-header-command{
  display:inline-flex;
  align-items:center;
  justify-content:center;
  min-width:auto !important;
}
@media (max-width: 900px){
  #wolfbbsCommandButton.wolfbbs-header-command{
    left:auto !important;
    right:auto !important;
    bottom:auto !important;
  }
}
#wolfbbsPaletteOverlay{
  padding:30px 16px;
  background:rgba(5,7,14,.62);
  backdrop-filter:blur(24px);
}
#wolfbbsPalette{
  max-width:1040px;
  border-radius:2rem;
}
#wolfbbsPaletteHeader{
  padding:1.2rem;
  background:linear-gradient(180deg,rgba(255,255,255,.07),rgba(255,255,255,.03));
  border-bottom:1px solid rgba(195,220,255,.12);
}
#wolfbbsPaletteHeader label{
  display:block;
  margin:0;
}
#wolfbbsPaletteHeader input{
  margin-top:.7rem;
}
#wolfbbsPaletteList{
  display:grid;
  gap:.75rem;
  max-height:min(54vh,540px);
  padding:1rem 1rem 1.1rem;
}
.wolfbbs-palette-item{
  align-items:center;
  padding:1rem 1.05rem;
  border-radius:1.3rem;
  border:1px solid rgba(195,220,255,.12);
  background:linear-gradient(180deg,rgba(255,255,255,.06),rgba(255,255,255,.025));
  box-shadow:inset 0 1px 0 rgba(255,255,255,.08);
}
.wolfbbs-palette-item:hover{
  background:linear-gradient(180deg,rgba(73,182,255,.18),rgba(255,83,213,.08));
}
.wolfbbs-palette-meta{
  color:rgba(223,232,255,.62);
}
.wolfbbs-palette-history{
  display:flex;
  flex-wrap:wrap;
  gap:.55rem;
  margin-top:.9rem;
}
.wolfbbs-palette-history button{
  padding:.5rem .8rem;
}
#wolfbbsOmnibarLabel{
  display:inline-flex;
  align-items:center;
  gap:.5rem;
  color:#d3e8ff;
  font-size:.82rem;
  font-weight:800;
  letter-spacing:.08em;
  text-transform:uppercase;
}
#wolfbbsOmnibarLabel::before{
  content:"";
  width:10px;
  height:10px;
  border-radius:50%;
  background:linear-gradient(135deg,var(--accent),var(--accent-2));
  box-shadow:0 0 16px rgba(73,182,255,.5);
}
.wolfbbs-omnibar-hint{
  display:block;
  margin-top:.5rem;
  color:rgba(227,234,255,.64);
  font-size:.93rem;
}
.wolfbbs-omnibar-module-grid{
  display:grid;
  grid-template-columns:repeat(12,minmax(0,1fr));
  gap:.75rem;
  padding:1rem 1rem 0;
}
.wolfbbs-omnibar-module{
  grid-column:span 4;
  width:100%;
  padding:1rem;
  text-align:left;
  border-radius:1.35rem;
  border:1px solid rgba(195,220,255,.12);
  background:linear-gradient(180deg,rgba(255,255,255,.07),rgba(255,255,255,.03));
  color:var(--text);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.08), 0 16px 34px rgba(0,0,0,.18);
}
.wolfbbs-omnibar-module strong{
  display:block;
  margin-bottom:.35rem;
}
.wolfbbs-omnibar-module span{
  display:block;
  color:rgba(226,234,255,.64);
}
.wolfbbs-spatial-card{
  display:grid;
  grid-template-columns:minmax(0,1fr) minmax(360px,1.15fr);
  gap:1.25rem;
  padding:1.4rem;
}
.wolfbbs-spatial-copy{
  display:grid;
  align-content:start;
  gap:.95rem;
}
.wolfbbs-spatial-copy p{
  color:rgba(228,236,255,.72);
}
.wolfbbs-spatial-copy ul{
  margin:0;
  padding-left:1.15rem;
}
.wolfbbs-spatial-shell{
  position:relative;
  min-height:360px;
  border-radius:calc(var(--radius) - .4rem);
  overflow:hidden;
  border:1px solid rgba(195,220,255,.12);
  background:linear-gradient(180deg,rgba(6,9,18,.8),rgba(10,13,24,.98));
  box-shadow:inset 0 1px 0 rgba(255,255,255,.05);
}
.wolfbbs-webgl-canvas{
  position:absolute;
  inset:0;
  width:100%;
  height:100%;
  display:block;
}
.wolfbbs-spatial-stage{
  position:absolute;
  inset:0;
  display:grid;
  place-items:center;
  perspective:1600px;
}
.wolfbbs-spatial-terminal{
  position:relative;
  width:min(74%,420px);
  aspect-ratio:1.26;
  transform-style:preserve-3d;
  transform:rotateX(calc(56deg + var(--wolfbbs-orbit-y,0deg))) rotateY(calc(-26deg + var(--wolfbbs-orbit-x,0deg))) translateZ(0);
  animation:wolfbbsSpatialFloat 7s ease-in-out infinite;
  will-change:transform;
}
.wolfbbs-spatial-terminal-face{
  position:absolute;
  inset:0;
  border-radius:1.6rem;
  border:1px solid rgba(190,215,255,.16);
  background:linear-gradient(180deg,rgba(23,31,48,.92),rgba(9,12,22,.96));
  box-shadow:inset 0 1px 0 rgba(255,255,255,.08);
}
.wolfbbs-spatial-terminal-face.screen{
  padding:1rem;
  transform:translateZ(58px);
  background:
    linear-gradient(180deg,rgba(19,27,43,.94),rgba(7,10,18,.98)),
    radial-gradient(circle at 20% 0%, rgba(73,182,255,.12), transparent 30%);
}
.wolfbbs-spatial-terminal-face.back{
  transform:translateZ(-58px) rotateY(180deg);
}
.wolfbbs-spatial-terminal-face.top{
  inset:0 0 auto 0;
  height:116px;
  transform-origin:top;
  transform:rotateX(-90deg) translateY(-58px);
}
.wolfbbs-spatial-terminal-face.bottom{
  inset:auto 0 0 0;
  height:116px;
  transform-origin:bottom;
  transform:rotateX(90deg) translateY(58px);
}
.wolfbbs-spatial-terminal-face.side-left{
  inset:0 auto 0 0;
  width:116px;
  transform-origin:left;
  transform:rotateY(-90deg) translateX(-58px);
}
.wolfbbs-spatial-terminal-face.side-right{
  inset:0 0 0 auto;
  width:116px;
  transform-origin:right;
  transform:rotateY(90deg) translateX(58px);
}
.wolfbbs-spatial-screen-ui{
  display:grid;
  gap:.75rem;
  height:100%;
}
.wolfbbs-spatial-screen-bar{
  display:flex;
  align-items:center;
  justify-content:space-between;
  gap:.75rem;
  font-size:.8rem;
  color:rgba(228,236,255,.72);
}
.wolfbbs-spatial-screen-row{
  display:grid;
  grid-template-columns:1.2fr .8fr;
  gap:.75rem;
  flex:1;
}
.wolfbbs-spatial-screen-stack{
  display:grid;
  gap:.75rem;
}
.wolfbbs-spatial-screen-panel{
  padding:.85rem;
  border-radius:1rem;
  border:1px solid rgba(188,214,255,.12);
  background:linear-gradient(180deg,rgba(255,255,255,.06),rgba(255,255,255,.02));
}
.wolfbbs-spatial-screen-panel strong{
  display:block;
  margin-bottom:.35rem;
}
.wolfbbs-spatial-caption{
  position:absolute;
  left:1rem;
  right:1rem;
  bottom:1rem;
  display:flex;
  justify-content:space-between;
  gap:1rem;
  padding:.85rem 1rem;
  border-radius:1rem;
  border:1px solid rgba(188,214,255,.1);
  background:linear-gradient(180deg,rgba(255,255,255,.05),rgba(255,255,255,.015));
  color:rgba(228,236,255,.74);
}
.wolfbbs-spatial-caption strong{
  color:#ffffff;
}
.wolfbbs-reveal{
  opacity:0;
  transform:translateY(18px) scale(.985);
  transition:
    opacity .55s ease,
    transform .75s cubic-bezier(.2,.75,.2,1);
  transition-delay:var(--wolfbbs-reveal-delay,0ms);
}
.wolfbbs-reveal.is-visible{
  opacity:1;
  transform:translateY(0) scale(1);
}
.wolfbbs-kinetic-title{
  font-weight:var(--wolfbbs-hero-weight,780);
  font-variation-settings:"wght" var(--wolfbbs-hero-weight,780);
  letter-spacing:calc(.012em + var(--wolfbbs-hero-shift,0) * .006em);
  transform:
    translate3d(calc(var(--wolfbbs-hero-x,0) * 1px),calc(var(--wolfbbs-hero-y,0) * 1px),0)
    scale(calc(1 + var(--wolfbbs-hero-scale,0)));
  text-shadow:
    0 0 calc(20px + (var(--wolfbbs-hero-glow,0) * 18px)) rgba(73,182,255,.18),
    0 0 calc(36px + (var(--wolfbbs-hero-glow,0) * 14px)) rgba(255,83,213,.12);
  will-change:transform;
}
button,
.wolfbbs-action-card,
.wolfbbs-palette-item,
.wolfbbs-omnibar-module,
.wolfbbs-hero-chip{
  transition:
    transform .22s ease,
    box-shadow .22s ease,
    border-color .22s ease,
    background .22s ease;
}
.wolfbbs-morph-active{
  transform:translateY(1px) scale(.97) !important;
  box-shadow:inset 0 12px 28px rgba(255,255,255,.08), 0 12px 24px rgba(0,0,0,.18) !important;
  border-color:rgba(255,255,255,.32) !important;
}
@keyframes wolfbbsSpatialFloat{
  0%,100%{
    transform:rotateX(calc(56deg + var(--wolfbbs-orbit-y,0deg))) rotateY(calc(-26deg + var(--wolfbbs-orbit-x,0deg))) translateY(0);
  }
  50%{
    transform:rotateX(calc(54deg + var(--wolfbbs-orbit-y,0deg))) rotateY(calc(-22deg + var(--wolfbbs-orbit-x,0deg))) translateY(-10px);
  }
}
@supports (grid-template-columns:subgrid){
  .wolfbbs-grid > .wolfbbs-card,
  .wolfbbs-grid > .wolfbbs-helper-card,
  .wolfbbs-card-grid > .wolfbbs-card,
  .wolfbbs-helper-grid > .wolfbbs-helper-card{
    display:grid;
    grid-template-columns:subgrid;
  }
  .wolfbbs-grid > .wolfbbs-card > *,
  .wolfbbs-grid > .wolfbbs-helper-card > *,
  .wolfbbs-card-grid > .wolfbbs-card > *,
  .wolfbbs-helper-grid > .wolfbbs-helper-card > *{
    grid-column:1 / -1;
  }
}
@container (max-width: 56rem){
  .wolfbbs-grid > *,
  .wolfbbs-card-grid > *,
  .wolfbbs-kpi-grid > *,
  .wolfbbs-action-grid > *,
  .wolfbbs-helper-grid > *,
  .wolfbbs-omnibar-module{
    grid-column:1 / -1;
  }
}
@media (max-width: 820px){
  body{
    width:calc(100% - 1.2rem);
    padding:16px 0 84px;
  }
  .wolfbbs-chat-shell{
    grid-template-columns:1fr;
  }
  .wolfbbs-room-list{
    display:grid;
    grid-template-columns:repeat(auto-fit,minmax(220px,1fr));
  }
  .wolfbbs-chat-header-main{
    flex-direction:column;
  }
  .wolfbbs-chat-header-stats{
    justify-content:flex-start;
  }
  .wolfbbs-chat-toolbar{
    grid-template-columns:1fr;
  }
  .wolfbbs-page-hero,
  .wolfbbs-spatial-card{
    grid-template-columns:1fr;
    padding:1.35rem;
  }
  .wolfbbs-page-hero-meta{
    justify-content:flex-start;
  }
  .wolfbbs-main{
    gap:1rem;
  }
  .wolfbbs-grid,
  .wolfbbs-card-grid,
  .wolfbbs-kpi-grid,
  .wolfbbs-action-grid,
  .wolfbbs-helper-grid,
  .wolfbbs-omnibar-module-grid{
    grid-template-columns:repeat(6,minmax(0,1fr));
  }
  .wolfbbs-grid > *,
  .wolfbbs-card-grid > *,
  .wolfbbs-kpi-grid > *,
  .wolfbbs-action-grid > *,
  .wolfbbs-helper-grid > *,
  .wolfbbs-omnibar-module{
    grid-column:span 3;
  }
  p.wolfbbs-nav-row,body > p:has(> a){padding:9px 10px}
  input[type=text],input[type=password],input[type=email],input[type=number],input[type=url],input[type=search],select,textarea{
    width:100%;
  }
  .wolfbbs-table-wrap table{
    min-width:560px;
  }
  .wolfbbs-chat-layout{
    grid-template-columns:1fr;
  }
  .wolfbbs-spatial-terminal{
    width:min(82%,360px);
  }
  .wolfbbs-spatial-screen-row{
    grid-template-columns:1fr;
  }
  .wolfbbs-spatial-caption{
    flex-direction:column;
    align-items:flex-start;
  }
  #wolfbbsActionDock{
    bottom:86px;
  }
  #wolfbbsCommandButton{
    left:10px;
    right:10px;
    bottom:14px;
    justify-content:center;
  }
}
@media (max-width: 560px){
  h1{font-size:1.56rem}
  h2{font-size:1.12rem}
  .wolfbbs-card,.wolfbbs-kpi-card,.wolfbbs-helper-card,.wolfbbs-action-card,.wolfbbs-primer{padding:14px 14px}
  .wolfbbs-chat-primer{
    padding:14px 15px;
    border-radius:18px;
  }
  .wolfbbs-room-list{
    grid-template-columns:1fr;
  }
  .wolfbbs-chat-pane{
    min-height:420px;
    height:60vh;
    padding:14px;
  }
  .wolfbbs-chat-line{
    gap:10px;
  }
  .wolfbbs-chat-line-avatar{
    width:30px;
    height:30px;
    flex-basis:30px;
    border-radius:10px;
    font-size:.8rem;
  }
  .wolfbbs-chat-line-bubble{
    padding:11px 12px;
  }
  .wolfbbs-chat-line-head{
    flex-direction:column;
    align-items:flex-start;
    gap:4px;
  }
  .wolfbbs-chat-composer-form{
    grid-template-columns:1fr;
  }
  .wolfbbs-grid,
  .wolfbbs-card-grid,
  .wolfbbs-kpi-grid,
  .wolfbbs-action-grid,
  .wolfbbs-helper-grid,
  .wolfbbs-omnibar-module-grid{
    grid-template-columns:1fr;
  }
  .wolfbbs-grid > *,
  .wolfbbs-card-grid > *,
  .wolfbbs-kpi-grid > *,
  .wolfbbs-action-grid > *,
  .wolfbbs-helper-grid > *,
  .wolfbbs-omnibar-module{
    grid-column:1 / -1;
  }
  #wolfbbsPaletteHeader{
    padding:1rem;
  }
  #wolfbbsPaletteList{
    padding:.75rem;
  }
  .wolfbbs-spatial-shell{
    min-height:300px;
  }
  .wolfbbs-spatial-terminal{
    width:min(88%,300px);
  }
}
body[data-route-profile="dense"] .wolfbbs-guide-strip,
body[data-route-profile="dense"] .wolfbbs-ux20-compass,
body[data-route-profile="dense"] .wolfbbs-scorecard,
body[data-route-profile="dense"] .wolfbbs-goal-coach,
body[data-route-profile="dense"] .wolfbbs-section-pin-rail{
  border-color:rgba(176,204,255,.14);
  background:linear-gradient(180deg,rgba(19,27,42,.82),rgba(9,13,24,.9));
  box-shadow:0 16px 38px rgba(0,0,0,.24);
}
body[data-route-profile="dense"] .wolfbbs-guide-strip{
  margin:6px 0 10px;
  padding:10px 12px;
}
body[data-route-profile="dense"] .wolfbbs-guide-actions a,
body[data-route-profile="dense"] .wolfbbs-section-nav .wolfbbs-section-toggle,
body[data-route-profile="dense"] .wolfbbs-section-nav-tools > summary{
  min-height:28px;
  padding:.45rem .8rem;
}
.wolfbbs-section-nav-compact{
  gap:.55rem;
}
.wolfbbs-section-nav-link-hidden{
  display:none !important;
}
.wolfbbs-section-nav-more{
  order:-1;
}
#wolfbbsMobileToolsButton{
  display:none;
  position:fixed;
  right:14px;
  bottom:108px;
  z-index:44;
  min-height:40px;
  padding:.8rem 1rem;
  border-radius:999px;
  border:1px solid rgba(173,222,255,.34);
  background:linear-gradient(180deg,rgba(26,37,58,.96),rgba(10,15,27,.96));
  color:#eef7ff;
  box-shadow:0 18px 42px rgba(0,0,0,.34);
}
#wolfbbsMobileToolsOverlay{
  position:fixed;
  inset:0;
  z-index:74;
  display:none;
  padding:1rem;
  background:rgba(5,9,18,.72);
  backdrop-filter:blur(18px);
  -webkit-backdrop-filter:blur(18px);
}
#wolfbbsMobileToolsOverlay.active{
  display:block;
}
#wolfbbsMobileToolsPanel{
  position:absolute;
  left:1rem;
  right:1rem;
  bottom:1rem;
  max-height:min(78vh,760px);
  overflow:auto;
  padding:1.1rem;
  border-radius:1.6rem;
  border:1px solid rgba(185,210,255,.15);
  background:linear-gradient(180deg,rgba(24,33,52,.96),rgba(10,14,24,.98));
  box-shadow:0 28px 78px rgba(0,0,0,.45);
}
#wolfbbsMobileToolsList{
  display:grid;
  gap:.95rem;
  margin-top:.9rem;
}
.wolfbbs-mobile-tools-group{
  display:grid;
  gap:.6rem;
}
.wolfbbs-mobile-tools-group strong{
  color:#f2f6ff;
  font-size:.82rem;
  letter-spacing:.04em;
  text-transform:uppercase;
}
.wolfbbs-mobile-tools-grid{
  display:grid;
  grid-template-columns:repeat(2,minmax(0,1fr));
  gap:.65rem;
}
.wolfbbs-mobile-tools-grid a,
.wolfbbs-mobile-tools-grid button{
  min-height:46px;
  justify-content:flex-start;
  text-align:left;
  padding:.8rem .95rem;
  border-radius:1rem;
  border:1px solid rgba(185,210,255,.14);
  background:linear-gradient(180deg,rgba(35,47,72,.92),rgba(18,24,39,.96));
  color:#eef7ff;
  box-shadow:none;
  text-decoration:none;
  font-size:.88rem;
}
.wolfbbs-mobile-tools-grid a:hover,
.wolfbbs-mobile-tools-grid button:hover{
  text-decoration:none;
  border-color:rgba(173,222,255,.32);
  background:linear-gradient(180deg,rgba(45,61,90,.94),rgba(20,28,45,.98));
}
@media (max-width: 820px){
  #wolfbbsActionDock,
  #wolfbbsNotesButton,
  #wolfbbsUXDiagButton,
  #wolfbbsFeedbackButton,
  #wolfbbsBugButton{
    display:none !important;
  }
  #wolfbbsMobileToolsButton{
    display:inline-flex;
    align-items:center;
    justify-content:center;
  }
  #wolfbbsCommandButton{
    left:14px;
    right:auto;
    bottom:108px;
  }
  .wolfbbs-section-pin-rail{
    padding:.6rem .8rem;
  }
}
@media (max-width: 560px){
  #wolfbbsCommandButton,
  #wolfbbsMobileToolsButton{
    min-width:calc(50% - 1.2rem);
  }
  .wolfbbs-mobile-tools-grid{
    grid-template-columns:1fr;
  }
}
@media (prefers-reduced-motion:reduce){
  *,*::before,*::after{
    animation:none !important;
    transition:none !important;
  }
}
</style>`
