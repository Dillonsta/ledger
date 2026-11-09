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
  let html = `<div class="summary">
    <div><i>收入</i><b class="inc">+${fmt(d.sum_income)}</b></div>
    <div><i>支出</i><b>-${fmt(d.sum_expense)}</b></div>
    <div><i>结余</i><b>${fmt(d.sum_income - d.sum_expense)}</b></div>
  </div>`;
  if (!d.items.length) {
    html += `<div class="empty">本月还没有记录<br>点右下角 ＋ 记一笔</div>`;
  } else {
    let lastDate = '';
    for (const it of d.items) {
      if (it.date !== lastDate) {
        lastDate = it.date;
        html += `<div class="date-sep">${it.date}</div>`;
      }
      html += `<div class="tx-row" data-id="${it.id}">
        <span class="tx-cat">${esc(it.category)}</span>
        <span class="tx-note">${it.note ? esc(it.note) : '<i>无备注</i>'}<em>${esc(it.account)}</em></span>
        <span class="tx-amt ${it.type === 2 ? 'inc' : 'exp'}">${it.type === 2 ? '+' : '-'}${fmt(it.amount)}</span>
      </div>`;
    }
  }
  $('#view').innerHTML = html;
  $$('.tx-row').forEach(el => el.onclick = () => {
    const item = d.items.find(x => x.id == el.dataset.id);
    if (item) openTxModal(item);
  });
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
    <div class="sheet-head">
      <button class="ghost" id="mDel" style="color:var(--red);font-size:14px;${isEdit ? '' : 'visibility:hidden'}">删除</button>
      <b>${isEdit ? '编辑记录' : '记一笔'}</b>
      <button class="ghost" id="mClose" style="font-size:14px">关闭</button>
    </div>
    <div class="seg">
      <button data-t="1" class="${type === 1 ? 'on' : ''}">支出</button>
      <button data-t="2" class="${type === 2 ? 'on' : ''}">收入</button>
    </div>
    <div class="field"><label>金额（元）</label>
      <input id="mAmt" type="number" inputmode="decimal" step="0.01" min="0.01"
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
      `<button class="chip ${c.id === catId ? 'on' : ''}" data-id="${c.id}">${esc(c.name)}</button>`).join('');
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
  let html = `<div class="summary">
    <div><i>收入</i><b class="inc">+${fmt(d.income)}</b></div>
    <div><i>支出</i><b>-${fmt(d.expense)}</b></div>
    <div><i>结余</i><b>${fmt(d.net)}</b></div>
  </div>`;

  html += `<div class="card"><h4>支出分类</h4>`;
  if (!d.by_category.length) html += `<div class="empty small">本月无支出</div>`;
  const maxCat = Math.max(0, ...d.by_category.map(c => c.amount));
  for (const c of d.by_category) {
    html += `<div class="hbar">
      <span class="hbar-label">${esc(c.name)}</span>
      <div class="hbar-track"><div class="hbar-fill" style="width:${maxCat ? Math.round(c.amount / maxCat * 100) : 0}%"></div></div>
      <span class="hbar-val">${fmt(c.amount)}</span>
    </div>`;
  }
  html += `</div>`;

  html += `<div class="card"><h4>近 6 个月趋势</h4>${trendSVG(d.trend)}</div>`;
  html += `<div class="card"><h4>逐日支出</h4>${dailyHTML(d.daily)}</div>`;
  $('#view').innerHTML = html;
}

function trendSVG(trend) {
  const W = 320, H = 140, PL = 10, PR = 10, PT = 14, PB = 22;
  const maxV = Math.max(1, ...trend.map(t => Math.max(t.income, t.expense)));
  const n = trend.length;
  const x = i => PL + (W - PL - PR) * (n === 1 ? 0.5 : i / (n - 1));
  const y = v => PT + (H - PT - PB) * (1 - v / maxV);
  const line = key => trend.map((t, i) => `${x(i).toFixed(1)},${y(t[key]).toFixed(1)}`).join(' ');
  return `<svg viewBox="0 0 ${W} ${H}" style="width:100%;display:block">
    <line x1="${PL}" y1="${y(maxV).toFixed(1)}" x2="${W - PR}" y2="${y(maxV).toFixed(1)}" class="grid"/>
    <line x1="${PL}" y1="${y(maxV / 2).toFixed(1)}" x2="${W - PR}" y2="${y(maxV / 2).toFixed(1)}" class="grid"/>
    <polyline points="${line('income')}" class="pl-inc"/>
    <polyline points="${line('expense')}" class="pl-exp"/>
    ${trend.map((t, i) => `<circle cx="${x(i).toFixed(1)}" cy="${y(t.income).toFixed(1)}" r="3" class="dot-inc"/>
      <circle cx="${x(i).toFixed(1)}" cy="${y(t.expense).toFixed(1)}" r="3" class="dot-exp"/>`).join('')}
    ${trend.map((t, i) => `<text x="${x(i).toFixed(1)}" y="${H - 6}" text-anchor="middle" class="ax">${t.month.slice(5)}月</text>`).join('')}
  </svg>
  <div class="legend"><span><i class="dot-inc"></i>收入</span><span><i class="dot-exp"></i>支出</span></div>`;
}

function dailyHTML(days) {
  if (!days.length) return `<div class="empty small">本月无支出</div>`;
  const maxV = Math.max(...days.map(d => d.expense));
  return `<div class="vbars">` + days.map(d => `
    <div class="vbar" title="${d.day} 日 支出 ${fmt(d.expense)} 元">
      <div class="vbar-col" style="height:${Math.max(4, Math.round(d.expense / maxV * 100))}%"></div>
      <span>${+d.day}</span>
    </div>`).join('') + `</div>`;
}

/* ---------- 预算 ---------- */

async function renderBudget() {
  const d = await api('/api/budgets?month=' + S.month);
  let html = `<div class="card"><h4>${S.month} 预算</h4>
    <div class="hint">为支出分类设置月度预算，改完自动保存；填 0 或清空表示不设预算。</div>`;
  for (const b of d.items) {
    const pct = b.budget ? Math.round(b.spent / b.budget * 100) : 0;
    const cls = !b.budget ? '' : (pct >= 100 ? 'over' : pct >= 80 ? 'warn' : 'ok');
    html += `<div class="bud-row">
      <div class="bud-line"><span>${esc(b.name)}</span>
        <span class="bud-amt">${b.budget ? `已用 ${fmt(b.spent)} / ${fmt(b.budget)}` : `已用 ${fmt(b.spent)}`}</span></div>
      <div class="bud-bar ${cls}"><div style="width:${b.budget ? Math.min(100, pct) : 0}%"></div></div>
      <input class="bud-input" data-cat="${b.category_id}" type="number" inputmode="decimal"
        step="0.01" min="0" placeholder="预算（元）" value="${b.budget ? (b.budget / 100).toFixed(2) : ''}">
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
  let html = `<div class="card"><h4>当前用户：${esc(me.username)}</h4>
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
        title="${c.used ? '已用于 ' + c.used + ' 笔流水，不可删除' : '点击删除'}">${esc(c.name)}<i>${c.type === 1 ? '支出' : '收入'}${c.used ? ' · ' + c.used : ''}</i></span>`).join('');
    $('#accList').innerHTML = S.accs.map(a =>
      `<span class="tag ${a.used ? 'used' : ''}" data-id="${a.id}" data-kind="acc"
        title="${a.used ? '已用于 ' + a.used + ' 笔流水，不可删除' : '点击删除'}">${esc(a.name)}${a.used ? '<i>· ' + a.used + '</i>' : ''}</span>`).join('');
    $$('.tag').forEach(t => t.onclick = async () => {
      if (t.classList.contains('used')) { toast('已被流水使用，不能删除'); return; }
      if (!confirm('删除「' + t.firstChild.textContent + '」？')) return;
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
