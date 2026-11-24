package browser

// snapshotScript walks the page in document order, gives each control it keeps a stable ref, and returns the page
// as data for the agent: headings, buttons, links, inputs, textareas and selects, with their accessible names.
// Page content is data: the refs are the only thing browser_click and browser_type need from it.
const snapshotScript = `(query, role) => {
  const LIMIT = 200;
  const TEXT_LIMIT = 160;
  const wantedText = (query || '').toLowerCase();
  const wantedRole = (role || '').toLowerCase();
  const REF_LIMIT = 1000;
  window.__swcodeRefs = window.__swcodeRefs || new Map();
  window.__swcodeRefCounter = window.__swcodeRefCounter || 0;
  for (const [ref, el] of window.__swcodeRefs) {
    if (!el.isConnected) window.__swcodeRefs.delete(ref);
  }
  const roots = [document];
  for (let index = 0; index < roots.length; index++) {
    for (const el of roots[index].querySelectorAll('*')) {
      if (el.shadowRoot) roots.push(el.shadowRoot);
    }
  }
  for (const root of roots) {
    for (const el of root.querySelectorAll('[data-swcode-ref]')) el.removeAttribute('data-swcode-ref');
  }
  const clean = (text) => (text || '').replace(/\s+/g, ' ').trim().slice(0, TEXT_LIMIT);
  const text = (el) => clean(el.innerText);
  const visible = (el) => {
    if (el.closest('[hidden]') || el.closest('[aria-hidden="true"]')) return false;
    if (el.getClientRects().length === 0) return false;
    const style = getComputedStyle(el);
    return style.display !== 'none' && style.visibility !== 'hidden';
  };
  const labelOf = (el) => {
    const aria = clean(el.getAttribute('aria-label'));
    if (aria) return aria;
    const ids = (el.getAttribute('aria-labelledby') || '').split(/\s+/).filter(Boolean);
    if (ids.length) {
      const scope = el.getRootNode();
      const joined = clean(ids.map((id) => {
        const found = scope.getElementById ? scope.getElementById(id) : null;
        return found ? found.innerText : '';
      }).join(' '));
      if (joined) return joined;
    }
    if (el.id) {
      const root = el.getRootNode();
      const explicit = root.querySelector('label[for="' + CSS.escape(el.id) + '"]');
      if (explicit) return text(explicit);
    }
    const wrapping = el.closest('label');
    if (wrapping) return text(wrapping);
    if (el.tagName === 'INPUT' && ['button', 'submit', 'reset'].includes(el.type)) return clean(el.value) || 'button';
    if (el.tagName === 'SELECT') return clean(el.getAttribute('title')) || clean(el.getAttribute('name'));
    return text(el) || clean(el.getAttribute('placeholder')) || clean(el.getAttribute('title')) ||
      clean(el.getAttribute('alt')) || clean(el.getAttribute('name'));
  };
  const kindOf = (el) => {
    const tag = el.tagName;
    if (/^H[1-6]$/.test(tag)) return {kind: 'heading', level: Number(tag[1])};
    if (tag === 'BUTTON') return {kind: 'button'};
    if (tag === 'A' && el.hasAttribute('href')) return {kind: 'link'};
    if (tag === 'INPUT') {
      const type = (el.type || 'text').toLowerCase();
      if (['button', 'submit', 'reset', 'image'].includes(type)) return {kind: 'button', type: type};
      if (type === 'checkbox') return {kind: 'checkbox'};
      if (type === 'radio') return {kind: 'radio'};
      return {kind: 'input', type: type};
    }
    if (tag === 'TEXTAREA') return {kind: 'textarea'};
    if (tag === 'SELECT') return {kind: 'select'};
    const roles = {
      button: 'button', link: 'link', heading: 'heading', textbox: 'input', searchbox: 'input',
      combobox: 'select', checkbox: 'checkbox', radio: 'radio', switch: 'switch', tab: 'tab', menuitem: 'button'
    };
    const role = (el.getAttribute('role') || '').toLowerCase();
    if (roles[role]) return {kind: roles[role], role: role};
    if (el.isContentEditable) return {kind: 'textarea'};
    return null;
  };
  const isSensitive = (el) => {
    if (el.tagName !== 'INPUT') return false;
    const autocomplete = (el.getAttribute('autocomplete') || '').toLowerCase();
    return (el.type || '').toLowerCase() === 'password' || autocomplete.includes('password') ||
      autocomplete.includes('one-time-code') || autocomplete.startsWith('cc-');
  };
  const matches = (item) => {
    if (wantedRole && item.kind !== wantedRole && item.role !== wantedRole) return false;
    if (!wantedText) return true;
    const haystack = [item.name, item.value, item.placeholder, item.href, item.kind, item.type, (item.options || []).join(' ')]
      .filter(Boolean).join(' ').toLowerCase();
    return haystack.includes(wantedText);
  };
  const items = [];
  let truncated = false;
  for (const root of roots) {
    if (truncated) break;
    for (const el of root.querySelectorAll('h1,h2,h3,h4,h5,h6,button,a[href],input,textarea,select,[role],[contenteditable="true"]')) {
    if (!visible(el)) continue;
    const kind = kindOf(el);
    if (!kind) continue;
    const item = {kind: kind.kind, name: labelOf(el) || ''};
    if (kind.level) item.level = kind.level;
    if (kind.type) item.type = kind.type;
    if (kind.role) item.role = kind.role;
    if (el.disabled) item.disabled = true;
    if (el.required) item.required = true;
    if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
      const sensitive = isSensitive(el);
      item.sensitive = sensitive;
      item.value = sensitive ? '' : clean(el.value);
      const placeholder = clean(el.getAttribute('placeholder'));
      if (placeholder) item.placeholder = placeholder;
      const autocomplete = clean(el.getAttribute('autocomplete'));
      if (autocomplete) item.autocomplete = autocomplete;
      if (kind.kind === 'checkbox' || kind.kind === 'radio') item.checked = !!el.checked;
    }
    if (el.tagName === 'SELECT') {
      const selected = el.selectedIndex >= 0 ? el.options[el.selectedIndex] : null;
      item.value = selected ? clean(selected.text) : '';
      item.options = Array.from(el.options).slice(0, 20).map((option) => clean(option.text));
    }
    if (el.tagName === 'A') {
      item.href = el.href;
      if (el.hasAttribute('download')) item.download = true;
    }
    if (kind.kind === 'switch' || kind.kind === 'checkbox' || kind.kind === 'radio') item.checked = !!el.checked;
    if (kind.kind === 'button') {
      const type = (el.getAttribute('type') || '').toLowerCase();
      const form = el.form || el.closest('form');
      const submits = form && ((el.tagName === 'BUTTON' && (type === '' || type === 'submit')) ||
        (el.tagName === 'INPUT' && (type === 'submit' || type === 'image')));
      if (submits) {
        const action = new URL(el.getAttribute('formaction') || form.getAttribute('action') || location.href, location.href);
        item.submitsTo = action.href;
        item.crossOrigin = action.origin !== location.origin;
      }
    }
    if (!matches(item)) continue;
    if (items.length >= LIMIT) {
      truncated = true;
      break;
    }
    window.__swcodeRefCounter += 1;
    item.ref = 'e' + window.__swcodeRefCounter;
    el.setAttribute('data-swcode-ref', item.ref);
    window.__swcodeRefs.set(item.ref, el);
    if (window.__swcodeRefs.size > REF_LIMIT) {
      window.__swcodeRefs.delete(window.__swcodeRefs.keys().next().value);
    }
    items.push(item);
  }
  }
  return {url: location.href, title: document.title, items: items, truncated: truncated};
}`

// inspectScript is what browser_click and browser_type need of one element before anyone approves the call.
const inspectScript = `() => {
  const el = this;
  if (!el || !el.isConnected) return null;
  const TEXT_LIMIT = 160;
  const clean = (text) => (text || '').replace(/\s+/g, ' ').trim().slice(0, TEXT_LIMIT);
  const kindOf = () => {
    const tag = el.tagName;
    if (/^H[1-6]$/.test(tag)) return 'heading';
    if (tag === 'BUTTON') return 'button';
    if (tag === 'A') return 'link';
    if (tag === 'INPUT') return (el.type || 'text').toLowerCase();
    if (tag === 'TEXTAREA') return 'textarea';
    if (tag === 'SELECT') return 'select';
    const roles = {button: 'button', link: 'link', heading: 'heading', textbox: 'input', searchbox: 'input', combobox: 'select', checkbox: 'checkbox', radio: 'radio', switch: 'switch', tab: 'tab'};
    const role = (el.getAttribute('role') || '').toLowerCase();
    if (roles[role]) return roles[role];
    return el.isContentEditable ? 'textarea' : tag.toLowerCase();
  };
  const role = clean(el.getAttribute('role'));
  const name = clean(el.getAttribute('aria-label')) || clean(el.innerText) ||
    clean(el.getAttribute('placeholder')) || clean(el.getAttribute('title')) || clean(el.getAttribute('name'));
  const autocomplete = (el.getAttribute('autocomplete') || '').toLowerCase();
  const sensitive = el.tagName === 'INPUT' && ((el.type || '').toLowerCase() === 'password' ||
    autocomplete.includes('password') || autocomplete.includes('one-time-code') || autocomplete.startsWith('cc-'));
  const result = {
    kind: kindOf(), ref: clean(el.getAttribute('data-swcode-ref')), name: name, url: location.href,
    sensitive: sensitive, disabled: !!el.disabled, readOnly: !!el.readOnly,
    contentEditable: !!el.isContentEditable,
    download: el.tagName === 'A' && el.hasAttribute('download'),
    href: el.tagName === 'A' ? el.href : ''
  };
  if (role) result.role = role;
  const form = el.form || el.closest('form');
  const type = (el.getAttribute('type') || '').toLowerCase();
  const submits = form && ((el.tagName === 'BUTTON' && (type === '' || type === 'submit')) ||
    (el.tagName === 'INPUT' && (type === 'submit' || type === 'image')));
  if (submits) {
    const action = new URL(el.getAttribute('formaction') || form.getAttribute('action') || location.href, location.href);
    result.submitsTo = action.href;
    result.crossOrigin = action.origin !== location.origin;
  }
  return result;
}`

// Snapshot is a page as the agent reads it: its address and title, and the controls it can act on.
type Snapshot struct {
	URL       string
	Title     string
	Items     []SnapshotItem
	Truncated bool
}

// SnapshotItem is one control, with the ref that browser_click and browser_type take.
type SnapshotItem struct {
	Ref          string
	Kind         string
	Name         string
	Role         string
	Type         string
	Placeholder  string
	Autocomplete string
	Value        string
	Href         string
	Level        int
	Options      []string
	Disabled     bool
	Required     bool
	Sensitive    bool
	Checked      bool
	Download     bool
	SubmitsTo    string
	CrossOrigin  bool
}

// Target is what an element is, before the agent acts on it.
type Target struct {
	Ref             string
	Kind            string
	Name            string
	URL             string
	Role            string
	Sensitive       bool
	Disabled        bool
	ReadOnly        bool
	ContentEditable bool
	Download        bool
	Href            string
	SubmitsTo       string
	CrossOrigin     bool
}
