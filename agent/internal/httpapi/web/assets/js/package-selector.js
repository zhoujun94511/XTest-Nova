(() => {
  'use strict';

  const targetSelector = '[data-package-select]';
  const filterSelector = '[data-package-filter]';
  /** @type {Map<string, {packageName: string, label: string}>} */
  const packageOptions = new Map();
  let selected = '';
  let onChange = () => {};

  const normalize = item => typeof item === 'string'
    ? {packageName: item, label: item}
    : {packageName: item.packageName || '', label: item.name || item.label || item.packageName || ''};

  const appendOptions = (select, emptyLabel) => {
    const current = select.matches(targetSelector) ? selected : select.value;
    select.replaceChildren(new Option(emptyLabel, ''));
    packageOptions.forEach(item => {
      select.append(new Option(item.label === item.packageName ? item.packageName : `${item.label} · ${item.packageName}`, item.packageName));
    });
    select.value = current;
  };

  const render = () => {
    document.querySelectorAll(targetSelector).forEach(select => appendOptions(select, '请选择目标应用'));
    document.querySelectorAll(filterSelector).forEach(select => appendOptions(select, '全部应用'));
  };

  const setValue = (value, notify = false) => {
    selected = String(value || '').trim();
    document.querySelectorAll(targetSelector).forEach(select => { select.value = selected; });
    if (notify) onChange(selected);
  };

  document.addEventListener('change', event => {
    if (!event.target.matches(targetSelector)) return;
    setValue(event.target.value);
    onChange(selected);
  });

  window.NovaPackageSelector = Object.freeze({
    configure(callback) { onChange = typeof callback === 'function' ? callback : () => {}; },
    setPackages(value) {
      packageOptions.clear();
      (Array.isArray(value) ? value : []).map(normalize).filter(item => item.packageName).forEach(item => packageOptions.set(item.packageName, item));
      render();
    },
    setValue,
    getValue() { return selected; }
  });
})();
