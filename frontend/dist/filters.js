// Filtros e navegação da revisão. São preferências de quem está revisando,
// então ficam só no navegador: o estado vai em classes do <html> (que o HTMX
// nunca troca) e o CSS esconde o que não interessa. Sobrevive ao polling.
(() => {
  const root = document.documentElement;
  const FILTERS_KEY = 'decrivo:filters';
  const DEFAULTS = {
    files: 'all', // all | commented | pending
    sev: 'all', // criticidade mínima: all | minor | major
    showContext: true,
    hideTests: false,
    hideGenerated: false,
    hideViewed: false,
    hideJudged: false,
    hidePraise: false,
    ignoreWs: false,
  };

  // localStorage pode falhar (modo privado, armazenamento bloqueado): nesse
  // caso os filtros continuam funcionando, só não são lembrados.
  const load = (key, fallback) => {
    try { return JSON.parse(localStorage.getItem(key)) ?? fallback; } catch { return fallback; }
  };
  const save = (key, value) => {
    try { localStorage.setItem(key, JSON.stringify(value)); } catch {}
  };

  let state = { ...DEFAULTS, ...load(FILTERS_KEY, {}) };
  const kebab = (s) => s.replace(/[A-Z]/g, (c) => '-' + c.toLowerCase());

  function applyFilters() {
    // Filtros de texto viram data-* no <html>; os de sim/não viram classes.
    for (const key of Object.keys(DEFAULTS)) {
      if (typeof DEFAULTS[key] === 'boolean') root.classList.toggle(kebab(key), state[key]);
      else root.dataset[key] = state[key];
    }
    syncControls();
    updateCount();
  }

  function syncControls() {
    for (const el of document.querySelectorAll('[data-filter]')) {
      const key = el.dataset.filter;
      if (el.type === 'checkbox') el.checked = state[key];
      else el.setAttribute('aria-pressed', String(state[key] === el.value));
    }
  }

  function setFilter(key, value) {
    state[key] = value;
    save(FILTERS_KEY, state);
    applyFilters();
  }

  // ---------- "Visto": por revisão, como no GitHub ----------
  const reviewId = () => document.getElementById('review')?.dataset.review;
  const viewedKey = () => 'decrivo:viewed:' + reviewId();
  const viewedSet = () => new Set(load(viewedKey(), []));

  function applyViewed() {
    if (!reviewId()) return;
    const viewed = viewedSet();
    for (const el of document.querySelectorAll('.file[data-path], .nav-item[data-path]')) {
      const isViewed = viewed.has(el.dataset.path);
      el.classList.toggle('viewed', isViewed);
      // O servidor sempre manda os arquivos abertos: recolhe os vistos só na
      // primeira vez que o elemento aparece, para respeitar quem reabriu um.
      if (el.tagName === 'DETAILS' && !el.dataset.viewedInit) {
        el.dataset.viewedInit = '1';
        if (isViewed) el.open = false;
      }
    }
    for (const btn of document.querySelectorAll('.viewed-btn')) {
      btn.setAttribute('aria-pressed', String(viewed.has(btn.dataset.viewedPath)));
    }
  }

  function toggleViewed(btn) {
    const path = btn.dataset.viewedPath;
    const viewed = viewedSet();
    const nowViewed = !viewed.has(path);
    nowViewed ? viewed.add(path) : viewed.delete(path);
    save(viewedKey(), [...viewed]);
    // Marcar como visto recolhe o arquivo; desmarcar abre de novo.
    const file = btn.closest('.file');
    if (file) file.open = !nowViewed;
    applyViewed();
    updateCount();
  }

  // ---------- contador e estado vazio ----------
  const isHidden = (el) => getComputedStyle(el).display === 'none';

  function updateCount() {
    const files = [...document.querySelectorAll('.files > .file')];
    const shown = files.filter((f) => !isHidden(f)).length;
    const out = document.querySelector('[data-count]');
    if (out) out.textContent = `Mostrando ${shown} de ${files.length} arquivos`;
    const empty = document.querySelector('.filter-empty');
    if (empty) empty.hidden = shown > 0 || files.length === 0;
  }

  // ---------- próximo / anterior pendente ----------
  let last = null;

  // Visível = nenhum ancestral escondido pelos filtros. <details> fechado não
  // conta: nesse caso o arquivo é aberto ao navegar até o achado.
  function reachable(el) {
    for (let e = el; e && e !== document.body; e = e.parentElement) {
      if (e.tagName !== 'DETAILS' && isHidden(e)) return false;
    }
    return true;
  }

  function goToPending(dir) {
    const all = [...document.querySelectorAll('.finding.v-pending')].filter(reachable);
    if (all.length === 0) return;

    let i = all.indexOf(last);
    if (i === -1) {
      // Sem referência (primeira vez ou o card foi re-renderizado): parte da tela atual.
      const bar = document.getElementById('statusbar')?.offsetHeight ?? 0;
      i = all.findIndex((f) => f.getBoundingClientRect().top > bar + 8);
      if (i === -1) i = 0;
      if (dir < 0) i = (i - 1 + all.length) % all.length;
    } else {
      i = (i + dir + all.length) % all.length;
    }

    const target = all[i];
    const file = target.closest('details.file');
    if (file && !file.open) file.open = true;
    target.scrollIntoView({ block: 'center', behavior: 'smooth' });
    target.classList.remove('flash');
    void target.offsetWidth; // reinicia a animação
    target.classList.add('flash');
    target.addEventListener('animationend', () => target.classList.remove('flash'), { once: true });
    last = target;
  }

  // ---------- eventos (delegados: os elementos são trocados pelo HTMX) ----------
  document.addEventListener('change', (e) => {
    const el = e.target.closest('input[type=checkbox][data-filter]');
    if (el) setFilter(el.dataset.filter, el.checked);
  });

  document.addEventListener('click', (e) => {
    const seg = e.target.closest('button[data-filter]');
    if (seg) return setFilter(seg.dataset.filter, seg.value);

    const viewed = e.target.closest('.viewed-btn');
    if (viewed) {
      e.preventDefault(); // o botão fica dentro do <summary>: não deixa abrir/fechar
      return toggleViewed(viewed);
    }

    if (e.target.closest('.js-next-pending')) return goToPending(1);
    if (e.target.closest('.js-reset-filters')) {
      state = { ...DEFAULTS };
      save(FILTERS_KEY, state);
      return applyFilters();
    }
    if (e.target.closest('.js-expand-all, .js-collapse-all')) {
      const open = !!e.target.closest('.js-expand-all');
      document.querySelectorAll('details.file').forEach((d) => { d.open = open; });
    }
  });

  document.addEventListener('keydown', (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.target instanceof Element && e.target.closest('input, textarea, select, [contenteditable]')) return;
    const key = e.key.toLowerCase();
    if (key === 'n') goToPending(1);
    else if (key === 'p') goToPending(-1);
  });

  // Depois de cada troca do HTMX (polling, veredito, recarga), reaplica o que
  // depende do DOM novo: estado dos controles, "vistos" e o contador.
  document.addEventListener('htmx:afterSettle', () => {
    syncControls();
    applyViewed();
    updateCount();
  });

  applyFilters();
})();
