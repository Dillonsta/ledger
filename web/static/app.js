'use strict';

const $ = s => document.querySelector(s);
const $$ = s => [...document.querySelectorAll(s)];

let token = localStorage.getItem('ledger_token') || '';
const S = { month: curMonth(), tab: 'list', cats: [], accs: [] };

/* ---------- 工具 ---------- */

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function fmt(cents) { return (cents / 100).toFixed(2); }
function pad(n) { return String(n).padStart(2, '0'); }
function curMonth() { const d = new Date(); return d.getFullYear() + '-' + pad(d.getMonth() + 1); }
function todayStr() { const d = new Date(); return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()); }
function shiftMonth(n) {
  const [y, m] = S.month.split('-').map(Number);
  const d = new Date(y, m - 1 + n, 1);
  S.month = d.getFullYear() + '-' + pad(d.getMonth() + 1);
}

const EMOJI = [
  ['餐饮', '🍜'], ['交通', '🚌'], ['购物', '🛍️'], ['居住', '🏠'], ['娱乐', '🎮'],
  ['医疗', '💊'], ['教育', '📚'], ['通讯', '📱'], ['工资', '💰'], ['奖金', '🏆'],
  ['理财', '📈'], ['兼职', '💼'], ['红包', '🧧'], ['旅行', '✈️'], ['宠物', '🐾'],
  ['运动', '⚽'], ['美容', '💅'], ['数码', '💻'], ['服饰', '👕'], ['日用', '🧺'],
];
function catEmoji(name) {
  for (const [k, e] of EMOJI) if (name.includes(k)) return e;
  return '🧾';
}
const TINTS = [
  ['#eef2ff', '#4c6fff'], ['#fff1ef', '#f06e72'], ['#eafaf3', '#21a366'],
  ['#fff7e8', '#e8930c'], ['#f3eeff', '#8b5cf6'], ['#e8f7fb', '#0e9fc0'],
  ['#fdeef7', '#d6468c'], ['#f0f5ea', '#5c9c3a'],
];
function catTint(id) {
  const t = TINTS[Math.abs(id) % TINTS.length];
  return `background:${t[0]}`;
}

let toastTimer = null;
function toast(msg) {
  const t = $('#toast');
  t.textContent = msg;
  t.classList.remove('hidden');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add('hidden'), 2200);
}

async function api(path, method = 'GET', body) {
  const res = await fetch(path, {
    method,
    headers: Object.assign(
      { 'Content-Type': 'application/json' },
      token ? { Authorization: 'Bearer ' + token } : {}
    ),
    body: body === undefined ? undefined : JSON.stringify(body)
  });
  if (res.status === 401) { location.replace('/login.html'); throw new Error('unauthorized'); }
  let data = {};
  try { data = await res.json(); } catch (e) { /* ignore */ }
  if (!res.ok) { toast(data.error || ('请求失败 ' + res.status)); throw new Error(data.error || 'fail'); }
  return data;
}

function heroStats(income, expense) {
  const net = income - expense;
  $('#heroStats').innerHTML = `
    <div class="col"><i>收入</i><b>+${fmt(income)}</b></div>
    <div class="col"><i>支出</i><b>-${fmt(expense)}</b></div>
    <div class="col"><i>结余</i><b class="${net < 0 ? 'net-negative' : ''}">${net < 0 ? '-' : ''}${fmt(Math.abs(net))}</b></div>`;
}

/* ---------- 初始化 ---------- */

async function loadMeta() {
  [S.cats, S.accs] = await Promise.all([api('/api/categories'), api('/api/accounts')]);
}

function bindUI() {
  $$('.tabbar button').forEach(b => b.onclick = () => {
    S.tab = b.dataset.tab;
    $$('.tabbar button').forEach(x => x.classList.toggle('on', x === b));
    $('#fab').classList.toggle('hidden', S.tab === 'settings');
    render();
  });
  $('#fab').onclick = () => openTxModal(null);
  $('#prevMonth').onclick = () => { shiftMonth(-1); render(); };
  $('#nextMonth').onclick = () => { shiftMonth(1); render(); };
}

async function render() {
  $('#curMonth').textContent = S.month;
  if (S.tab === 'list') return renderList();
  if (S.tab === 'report') return renderReport();
  if (S.tab === 'budget') return renderBudget();
  if (S.tab === 'settings') return renderSettings();
}

/* ---------- 明细 ---------- */

async function renderList() {
  const d = await api('/api/tx?month=' + S.month + '&size=100');
  heroStats(d.sum_income, d.sum_expense);
  let html = '';
  if (!d.items.length) {
    html = `<div class="empty"><span class="big">🍃</span>本月还没有记录<br>点右下角 ＋ 记一笔</div>`;
  } else {
    // 先按日期分组算好每日小计，再渲染（分隔条在组的上方）
    const groups = [];
    let cur = null;
    for (const it of d.items) {
      if (!cur || cur.date !== it.date) {
        cur = { date: it.date, items: [], sum: 0 };
        groups.push(cur);
      }
      cur.items.push(it);
      if (it.type === 1) cur.sum += it.amount;
    }
    for (const g of groups) {
      html += `<div class="date-sep"><span>${dateLabel(g.date)}</span><span>支出 ${fmt(g.sum)}</span></div>`;
      for (const it of g.items) {
        html += `<div class="tx-row" data-id="${it.id}">
          <span class="tx-icon" style="${catTint(it.category_id)}">${catEmoji(it.category)}</span>
          <span class="tx-mid">
            <span class="tx-cat">${esc(it.category)}</span>
            <span class="tx-note">${it.note ? esc(it.note) : '无备注'}<em>· ${esc(it.account)}</em></span>
          </span>
          <span class="tx-amt ${it.type === 2 ? 'inc' : ''}">${it.type === 2 ? '+' : '-'}${fmt(it.amount)}</span>
        </div>`;
      }
    }
  }
  $('#view').innerHTML = html;
  $$('.tx-row').forEach(el => el.onclick = () => {
    const item = d.items.find(x => x.id == el.dataset.id);
    if (item) openTxModal(item);
  });
}

function dateLabel(date) {
  const [y, m, dd] = date.split('-').map(Number);
  const now = new Date();
  if (y === now.getFullYear() && +m === now.getMonth() + 1 && dd === now.getDate()) return '今天';
  if (y === now.getFullYear() && +m === now.getMonth() + 1 && dd === now.getDate() - 1) return '昨天';
  return `${m}月${dd}日`;
}

/* ---------- 记一笔 / 编辑 ---------- */

function closeModal() { $('#modal').classList.add('hidden'); $('#modal').innerHTML = ''; }

function openTxModal(item) {
  const isEdit = !!item;
  const d = item || {
    type: 1, amount: 0, category_id: 0,
    account_id: (S.accs[0] || {}).id || 0,
    date: todayStr(), note: ''
  };
  let type = d.type, catId = d.category_id || 0;

  const m = $('#modal');
  m.innerHTML = `<div class="sheet">
    <div class="grabber"></div>
    <div class="sheet-head">
      <button class="ghost" id="mDel" style="color:var(--red);${isEdit ? '' : 'visibility:hidden'}">删除</button>
      <b>${isEdit ? '编辑记录' : '记一笔'}</b>
      <button class="ghost" id="mClose">关闭</button>
    </div>
    <div class="seg">
      <button data-t="1" class="${type === 1 ? 'on' : ''}">支出</button>
      <button data-t="2" class="${type === 2 ? 'on' : ''}">收入</button>
    </div>
    <div class="field"><label>金额（元）</label>
      <input id="mAmt" class="amount-input" type="number" inputmode="decimal" step="0.01" min="0.01"
        placeholder="0.00" value="${d.amount ? (d.amount / 100).toFixed(2) : ''}"></div>
    <div class="chips" id="mCats"></div>
    <div class="field"><label>账户</label>
      <select id="mAcc">${S.accs.map(a =>
        `<option value="${a.id}" ${a.id === d.account_id ? 'selected' : ''}>${esc(a.name)}</option>`).join('')}</select></div>
    <div class="field"><label>日期</label><input id="mDate" type="date" value="${d.date}"></div>
    <div class="field"><label>备注</label><input id="mNote" type="text" maxlength="100" value="${esc(d.note || '')}" placeholder="选填"></div>
    <button class="btn primary block" id="mSave">保 存</button>
  </div>`;
  m.classList.remove('hidden');

  function renderChips() {
    const cs = S.cats.filter(c => c.type === type);
    if (!catId || !cs.some(c => c.id === catId)) catId = cs.length ? cs[0].id : 0;
    $('#mCats').innerHTML = cs.map(c =>
      `<button class="chip ${c.id === catId ? 'on' : ''}" data-id="${c.id}">${catEmoji(c.name)} ${esc(c.name)}</button>`).join('');
    $$('#mCats .chip').forEach(c => c.onclick = () => {
      catId = +c.dataset.id;
      $$('#mCats .chip').forEach(x => x.classList.toggle('on', x === c));
    });
  }
  renderChips();

  $$('#modal .seg button').forEach(b => b.onclick = () => {
    type = +b.dataset.t; catId = 0;
    $$('#modal .seg button').forEach(x => x.classList.toggle('on', x === b));
    renderChips();
  });

  $('#mClose').onclick = closeModal;
  m.onclick = e => { if (e.target === m) closeModal(); };

  if (isEdit) $('#mDel').onclick = async () => {
    if (!confirm('删除这笔记录？')) return;
    await api('/api/tx/' + d.id, 'DELETE');
    closeModal(); render();
  };

  $('#mSave').onclick = async () => {
    const amt = Math.round(parseFloat($('#mAmt').value) * 100);
    if (!amt || amt <= 0 || isNaN(amt)) { toast('请输入正确的金额'); return; }
    if (!catId) { toast('请选择分类'); return; }
    const date = $('#mDate').value;
    if (!date) { toast('请选择日期'); return; }
    const body = {
      type, amount: amt, category_id: catId,
      account_id: +$('#mAcc').value, date,
      note: $('#mNote').value.trim()
    };
    try {
      if (isEdit) await api('/api/tx/' + d.id, 'PUT', body);
      else await api('/api/tx', 'POST', body);
      closeModal(); render();
    } catch (e) { /* toast 已提示 */ }
  };
}

/* ---------- 图表 ---------- */

async function renderReport() {
  const d = await api('/api/report?month=' + S.month);
  heroStats(d.income, d.expense);

  let html = `<div class="card"><h4>支出分类</h4>`;
  if (!d.by_category.length) {
    html += `<div class="empty small">本月无支出</div>`;
  } else {
    const maxCat = Math.max(...d.by_category.map(c => c.amount));
    const total = d.by_category.reduce((s, c) => s + c.amount, 0);
    html += d.by_category.map(c => {
      const cat = S.cats.find(x => x.name === c.name);
      return `<div class="hbar">
        <span class="hbar-icon" style="${catTint(cat ? cat.id : 0)}">${catEmoji(c.name)}</span>
        <span class="hbar-mid">
          <span class="hbar-top"><b>${esc(c.name)}</b><span>${fmt(c.amount)} · ${total ? Math.round(c.amount / total * 100) : 0}%</span></span>
          <span class="hbar-track"><span class="hbar-fill" style="display:block;width:${maxCat ? Math.max(4, Math.round(c.amount / maxCat * 100)) : 0}%"></span></span>
        </span>
      </div>`;
    }).join('');
  }
  html += `</div>`;

  html += `<div class="card"><h4>近 6 个月趋势</h4>${trendSVG(d.trend)}</div>`;
  html += `<div class="card"><h4>逐日支出</h4>${dailyHTML(d.daily)}</div>`;
  $('#view').innerHTML = html;
}

function trendSVG(trend) {
  const W = 320, H = 150, PL = 10, PR = 10, PT = 16, PB = 24;
  const maxV = Math.max(1, ...trend.map(t => Math.max(t.income, t.expense)));
  const n = trend.length;
  const x = i => PL + (W - PL - PR) * (n === 1 ? 0.5 : i / (n - 1));
  const y = v => PT + (H - PT - PB) * (1 - v / maxV);
  const pts = key => trend.map((t, i) => [x(i), y(t[key])]);
  const line = key => pts(key).map(p => `${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');
  const area = key => {
    const ps = pts(key);
    const top = ps.map(p => `${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' L');
    const last = ps[ps.length - 1], first = ps[0];
    return `M${first[0].toFixed(1)},${(H - PB).toFixed(1)} L${top} L${last[0].toFixed(1)},${(H - PB).toFixed(1)} Z`;
  };
  return `<svg viewBox="0 0 ${W} ${H}" style="width:100%;display:block">
    <defs>
      <linearGradient id="gInc" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#e5484d" stop-opacity=".22"/><stop offset="1" stop-color="#e5484d" stop-opacity="0"/>
      </linearGradient>
      <linearGradient id="gExp" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" stop-color="#4c6fff" stop-opacity=".22"/><stop offset="1" stop-color="#4c6fff" stop-opacity="0"/>
      </linearGradient>
    </defs>
    <line x1="${PL}" y1="${y(maxV).toFixed(1)}" x2="${W - PR}" y2="${y(maxV).toFixed(1)}" class="grid"/>
    <line x1="${PL}" y1="${y(maxV / 2).toFixed(1)}" x2="${W - PR}" y2="${y(maxV / 2).toFixed(1)}" class="grid"/>
    <path d="${area('income')}" class="area-inc"/>
    <path d="${area('expense')}" class="area-exp"/>
    <polyline points="${line('income')}" class="pl-inc"/>
    <polyline points="${line('expense')}" class="pl-exp"/>
    ${trend.map((t, i) => `<circle cx="${x(i).toFixed(1)}" cy="${y(t.income).toFixed(1)}" r="3.2" class="dot-inc"/>
      <circle cx="${x(i).toFixed(1)}" cy="${y(t.expense).toFixed(1)}" r="3.2" class="dot-exp"/>`).join('')}
    ${trend.map((t, i) => `<text x="${x(i).toFixed(1)}" y="${H - 7}" text-anchor="middle" class="ax">${t.month.slice(5)}月</text>`).join('')}
  </svg>
  <div class="legend"><span><i style="background:var(--red)"></i>收入</span><span><i style="background:#4c6fff"></i>支出</span></div>`;
}

function dailyHTML(days) {
  if (!days.length) return `<div class="empty small">本月无支出</div>`;
  const maxV = Math.max(...days.map(d => d.expense));
  return `<div class="vbars">` + days.map(d => `
    <div class="vbar" title="${d.day} 日支出 ${fmt(d.expense)} 元">
      <div class="vbar-col" style="height:${Math.max(3, Math.round(d.expense / maxV * 100))}%"></div>
      <span>${+d.day}</span>
    </div>`).join('') + `</div>`;
}

/* ---------- 预算 ---------- */

async function renderBudget() {
  const d = await api('/api/budgets?month=' + S.month);
  const totalBudget = d.items.reduce((s, b) => s + b.budget, 0);
  const totalSpent = d.items.reduce((s, b) => s + b.spent, 0);
  $('#heroStats').innerHTML = `
    <div class="col"><i>总预算</i><b>${totalBudget ? fmt(totalBudget) : '—'}</b></div>
    <div class="col"><i>已使用</i><b>${fmt(totalSpent)}</b></div>
    <div class="col"><i>使用率</i><b>${totalBudget ? Math.round(totalSpent / totalBudget * 100) + '%' : '—'}</b></div>`;

  let html = `<div class="card">
    <h4>${S.month} 分类预算</h4>
    <div class="hint">为支出分类设置月度预算，输入金额自动保存；填 0 或清空表示不设预算。</div>`;
  for (const b of d.items) {
    const pct = b.budget ? Math.round(b.spent / b.budget * 100) : 0;
    const cls = !b.budget ? '' : (pct >= 100 ? 'over' : pct >= 80 ? 'warn' : '');
    html += `<div class="bud-row">
      <span class="bud-icon" style="${catTint(b.category_id)}">${catEmoji(b.name)}</span>
      <span class="bud-mid">
        <span class="bud-line" style="display:flex">
          <b>${esc(b.name)}</b>
          <span>${b.budget ? `${fmt(b.spent)} / ${fmt(b.budget)}` : `已用 ${fmt(b.spent)}`}</span>
        </span>
        <span class="bud-bar ${cls}" style="display:block"><div style="width:${b.budget ? Math.min(100, pct) : 0}%"></div></span>
      </span>
      <input class="bud-input" data-cat="${b.category_id}" type="number" inputmode="decimal"
        step="0.01" min="0" placeholder="预算" value="${b.budget ? (b.budget / 100).toFixed(2) : ''}">
    </div>`;
  }
  html += `</div>`;
  $('#view').innerHTML = html;
  $$('.bud-input').forEach(inp => inp.onchange = async () => {
    const v = inp.value.trim();
    const cents = v === '' ? 0 : (Math.round(parseFloat(v) * 100) || 0);
    if (cents < 0) { toast('金额无效'); return; }
    try {
      await api('/api/budgets', 'PUT', { category_id: +inp.dataset.cat, month: S.month, amount: cents });
      toast('已保存');
      renderBudget();
    } catch (e) { /* toast 已提示 */ }
  });
}

/* ---------- 设置 ---------- */

async function renderSettings() {
  const me = await api('/api/me');
  $('#heroStats').innerHTML = `<div class="col" style="text-align:left"><i>当前用户</i><b>${esc(me.username)}</b></div>`;
  let html = `<div class="profile">
    <span class="avatar">${esc(me.username[0] || 'U').toUpperCase()}</span>
    <span><b>${esc(me.username)}</b><span>数据本地存储 · SQLite</span></span>
  </div>
  <div class="card"><h4>修改密码</h4>
    <div class="field"><label>原密码</label><input id="pwOld" type="password" autocomplete="current-password"></div>
    <div class="field"><label>新密码（至少 6 位）</label><input id="pwNew" type="password" autocomplete="new-password"></div>
    <button class="btn primary" id="pwBtn">修改密码</button>
  </div>
  <div class="card"><h4>分类管理</h4>
    <div class="addline">
      <input id="catName" maxlength="20" placeholder="新分类名">
      <select id="catType"><option value="1">支出</option><option value="2">收入</option></select>
      <button class="btn" id="catAdd">添加</button>
    </div>
    <div id="catList"></div>
  </div>
  <div class="card"><h4>账户管理</h4>
    <div class="addline">
      <input id="accName" maxlength="20" placeholder="新账户名（如：招行卡）">
      <button class="btn" id="accAdd">添加</button>
    </div>
    <div id="accList"></div>
  </div>`;
  $('#view').innerHTML = html;

  function renderTags() {
    $('#catList').innerHTML = S.cats.map(c =>
      `<span class="tag ${c.used ? 'used' : ''}" data-id="${c.id}" data-kind="cat"
        title="${c.used ? '已用于 ' + c.used + ' 笔流水，不可删除' : '点击删除'}">${catEmoji(c.name)} ${esc(c.name)}<i>${c.type === 1 ? '支出' : '收入'}${c.used ? ' · ' + c.used : ''}</i></span>`).join('');
    $('#accList').innerHTML = S.accs.map(a =>
      `<span class="tag ${a.used ? 'used' : ''}" data-id="${a.id}" data-kind="acc"
        title="${a.used ? '已用于 ' + a.used + ' 笔流水，不可删除' : '点击删除'}">💳 ${esc(a.name)}${a.used ? '<i>· ' + a.used + '</i>' : ''}</span>`).join('');
    $$('.tag').forEach(t => t.onclick = async () => {
      if (t.classList.contains('used')) { toast('已被流水使用，不能删除'); return; }
      const name = t.textContent.replace(/[^\u4e00-\u9fa5A-Za-z0-9].*$/, '').trim();
      if (!confirm('删除「' + name + '」？')) return;
      const p = t.dataset.kind === 'cat' ? '/api/categories/' + t.dataset.id : '/api/accounts/' + t.dataset.id;
      try {
        await api(p, 'DELETE');
        await loadMeta();
        renderSettings();
      } catch (e) { /* toast 已提示 */ }
    });
  }
  renderTags();

  $('#catAdd').onclick = async () => {
    const n = $('#catName').value.trim();
    if (!n) { toast('请输入分类名'); return; }
    try {
      await api('/api/categories', 'POST', { name: n, type: +$('#catType').value });
      $('#catName').value = '';
      await loadMeta(); renderSettings();
    } catch (e) { /* toast 已提示 */ }
  };
  $('#accAdd').onclick = async () => {
    const n = $('#accName').value.trim();
    if (!n) { toast('请输入账户名'); return; }
    try {
      await api('/api/accounts', 'POST', { name: n });
      $('#accName').value = '';
      await loadMeta(); renderSettings();
    } catch (e) { /* toast 已提示 */ }
  };
  $('#pwBtn').onclick = async () => {
    try {
      await api('/api/password', 'POST', { old_password: $('#pwOld').value, new_password: $('#pwNew').value });
      toast('密码已修改');
      $('#pwOld').value = ''; $('#pwNew').value = '';
    } catch (e) { /* toast 已提示 */ }
  };
}

/* ---------- 启动 ---------- */

(async function init() {
  try {
    await api('/api/me'); // 无 token 或过期会跳登录页
  } catch (e) {
    if (e.message === 'unauthorized') return;
  }
  bindUI();
  try {
    await loadMeta();
  } catch (e) { /* toast 已提示 */ }
  render();
})();
