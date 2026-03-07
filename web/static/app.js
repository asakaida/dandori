'use strict';

const UI_BASE = '/ui';
const API_BASE = '/v1';

// --- Utilities ---

function escapeHtml(str) {
  if (!str) return '';
  const el = document.createElement('span');
  el.textContent = str;
  return el.innerHTML;
}

function isValidTime(ts) {
  if (!ts) return false;
  const d = new Date(ts);
  return d.getTime() > 0;
}

function formatTime(ts) {
  if (!isValidTime(ts)) return '-';
  return new Date(ts).toLocaleString();
}

function decodeBytes(b64) {
  if (!b64) return null;
  try {
    const decoded = atob(b64);
    if (!decoded) return null;
    try {
      return JSON.parse(decoded);
    } catch {
      return decoded;
    }
  } catch {
    return null;
  }
}

function formatJson(obj) {
  if (obj === null || obj === undefined) return '-';
  if (typeof obj === 'string') return escapeHtml(obj);
  return escapeHtml(JSON.stringify(obj, null, 2));
}

// --- Status ---

const STATUS_CONFIG = {
  WORKFLOW_EXECUTION_STATUS_RUNNING:          { label: 'Running',          dot: 'fill-blue-500',   bg: 'bg-blue-100',   text: 'text-blue-700' },
  WORKFLOW_EXECUTION_STATUS_COMPLETED:        { label: 'Completed',        dot: 'fill-green-500',  bg: 'bg-green-100',  text: 'text-green-700' },
  WORKFLOW_EXECUTION_STATUS_FAILED:           { label: 'Failed',           dot: 'fill-red-500',    bg: 'bg-red-100',    text: 'text-red-700' },
  WORKFLOW_EXECUTION_STATUS_TERMINATED:       { label: 'Terminated',       dot: 'fill-yellow-500', bg: 'bg-yellow-100', text: 'text-yellow-800' },
  WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW: { label: 'Continued as New', dot: 'fill-purple-500', bg: 'bg-purple-100', text: 'text-purple-700' },
};

function statusBadge(status) {
  var c = STATUS_CONFIG[status] || { label: status || 'Unknown', dot: 'fill-gray-400', bg: 'bg-gray-100', text: 'text-gray-600' };
  return '<span class="inline-flex items-center gap-x-1.5 rounded-full ' + c.bg + ' px-2 py-1 text-xs font-medium ' + c.text + '">' +
    '<svg viewBox="0 0 6 6" aria-hidden="true" class="size-1.5 ' + c.dot + '"><circle r="3" cx="3" cy="3" /></svg>' +
    c.label +
    '</span>';
}

// --- Search Attributes ---

var OUTCOME_CONFIG = {
  waiting_for_human:     { label: 'Waiting for Operator', dot: 'fill-amber-500',  bg: 'bg-amber-100',  text: 'text-amber-800' },
  retrying:              { label: 'Retrying',             dot: 'fill-blue-500',   bg: 'bg-blue-100',   text: 'text-blue-800' },
  paid:                  { label: 'Paid',                 dot: 'fill-green-500',  bg: 'bg-green-100',  text: 'text-green-800' },
  cancelled_by_operator: { label: 'Cancelled by Operator', dot: 'fill-orange-500', bg: 'bg-orange-100', text: 'text-orange-800' },
};

function outcomeBadge(attrs) {
  if (!attrs || !attrs.outcome) return '';
  var val = attrs.outcome;
  var c = OUTCOME_CONFIG[val] || { label: val, dot: 'fill-gray-400', bg: 'bg-gray-100', text: 'text-gray-700' };
  return '<span class="inline-flex items-center gap-x-1.5 rounded-full ' + c.bg + ' px-2 py-1 text-xs font-medium ' + c.text + '">' +
    '<svg viewBox="0 0 6 6" aria-hidden="true" class="size-1.5 ' + c.dot + '"><circle r="3" cx="3" cy="3" /></svg>' +
    c.label +
  '</span>';
}

// --- Event Timeline ---

var EVENT_COLORS = {
  WorkflowExecutionStarted:        'bg-blue-500',
  WorkflowExecutionCompleted:      'bg-green-500',
  WorkflowExecutionFailed:         'bg-red-500',
  WorkflowExecutionTerminated:     'bg-yellow-500',
  ActivityTaskScheduled:           'bg-gray-400',
  ActivityTaskStarted:             'bg-blue-400',
  ActivityTaskCompleted:           'bg-green-400',
  ActivityTaskFailed:              'bg-red-400',
  TimerStarted:                    'bg-indigo-400',
  TimerFired:                      'bg-indigo-600',
  WorkflowSignaled:                'bg-purple-500',
  ChildWorkflowExecutionStarted:   'bg-cyan-500',
  ChildWorkflowExecutionCompleted: 'bg-cyan-600',
  WorkflowExecutionContinuedAsNew: 'bg-purple-400',
  SearchAttributesUpserted:        'bg-amber-500',
};

function eventIcon(eventType) {
  if (eventType.includes('Completed')) {
    return '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 text-white"><path d="M16.704 4.153a.75.75 0 01.143 1.052l-8 10.5a.75.75 0 01-1.127.075l-4.5-4.5a.75.75 0 011.06-1.06l3.894 3.893 7.48-9.817a.75.75 0 011.05-.143z" clip-rule="evenodd" fill-rule="evenodd" /></svg>';
  }
  if (eventType.includes('Failed') || eventType.includes('Terminated')) {
    return '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 text-white"><path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" /></svg>';
  }
  if (eventType.includes('Timer')) {
    return '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 text-white"><path d="M10 18a8 8 0 100-16 8 8 0 000 16zm.75-13a.75.75 0 00-1.5 0v5c0 .414.336.75.75.75h4a.75.75 0 000-1.5h-3.25V5z" clip-rule="evenodd" fill-rule="evenodd" /></svg>';
  }
  if (eventType.includes('Signal') || eventType.includes('SearchAttributes')) {
    return '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 text-white"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd" /></svg>';
  }
  if (eventType.includes('Started') || eventType.includes('Scheduled')) {
    return '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 text-white"><path d="M2 10a8 8 0 1116 0 8 8 0 01-16 0zm6.39-2.908a.75.75 0 01.766.027l3.5 2.25a.75.75 0 010 1.262l-3.5 2.25A.75.75 0 018 12.25v-4.5a.75.75 0 01.39-.658z" clip-rule="evenodd" fill-rule="evenodd" /></svg>';
  }
  return '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 text-white"><circle cx="10" cy="10" r="4" /></svg>';
}

// --- API ---

async function api(path) {
  var res = await fetch(API_BASE + path);
  if (!res.ok) {
    var body = await res.text();
    throw new Error('API ' + res.status + ': ' + body);
  }
  return res.json();
}

async function apiPost(path, body) {
  var res = await fetch(API_BASE + path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body || {}),
  });
  if (!res.ok) {
    var text = await res.text();
    throw new Error('API ' + res.status + ': ' + text);
  }
  return res.json();
}

// --- Toast Notification ---

var TOAST_ICONS = {
  success: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="size-6 text-green-400"><path d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" stroke-linecap="round" stroke-linejoin="round" /></svg>',
  error: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="size-6 text-red-400"><path d="m9.75 9.75 4.5 4.5m0-4.5-4.5 4.5M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" stroke-linecap="round" stroke-linejoin="round" /></svg>',
  warning: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="size-6 text-yellow-400"><path d="M12 9v3.75m9-.75a9 9 0 1 1-18 0 9 9 0 0 1 18 0Zm-9 3.75h.008v.008H12v-.008Z" stroke-linecap="round" stroke-linejoin="round" /></svg>',
};

function showToast(message, type) {
  var container = document.getElementById('toast-container');
  if (!container) {
    container = document.createElement('div');
    container.id = 'toast-container';
    container.setAttribute('aria-live', 'assertive');
    container.className = 'pointer-events-none fixed inset-0 z-50 flex items-end px-4 py-6 sm:items-start sm:p-6';
    container.innerHTML = '<div class="flex w-full flex-col items-center space-y-4 sm:items-end"></div>';
    document.body.appendChild(container);
  }
  var list = container.firstElementChild;
  var icon = TOAST_ICONS[type] || TOAST_ICONS.success;
  var toast = document.createElement('div');
  toast.className = 'pointer-events-auto w-full max-w-sm rounded-lg bg-white shadow-lg ring-1 ring-black/5 transition-all duration-300 ease-out';
  toast.innerHTML =
    '<div class="p-4">' +
      '<div class="flex items-start">' +
        '<div class="shrink-0">' + icon + '</div>' +
        '<div class="ml-3 w-0 flex-1 pt-0.5">' +
          '<p class="text-sm font-medium text-gray-900">' + escapeHtml(message) + '</p>' +
        '</div>' +
        '<div class="ml-4 flex shrink-0">' +
          '<button type="button" class="toast-close inline-flex rounded-md text-gray-400 hover:text-gray-500">' +
            '<span class="sr-only">Close</span>' +
            '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5"><path d="M6.28 5.22a.75.75 0 0 0-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 1 0 1.06 1.06L10 11.06l3.72 3.72a.75.75 0 1 0 1.06-1.06L11.06 10l3.72-3.72a.75.75 0 0 0-1.06-1.06L10 8.94 6.28 5.22Z" /></svg>' +
          '</button>' +
        '</div>' +
      '</div>' +
    '</div>';
  list.appendChild(toast);
  toast.querySelector('.toast-close').addEventListener('click', function() {
    toast.style.opacity = '0';
    setTimeout(function() { toast.remove(); }, 300);
  });
  setTimeout(function() {
    toast.style.opacity = '0';
    setTimeout(function() { toast.remove(); }, 300);
  }, 4000);
}

var DIALOG_ICONS = {
  warning: '<div class="mx-auto flex size-12 shrink-0 items-center justify-center rounded-full bg-yellow-100 sm:mx-0 sm:size-10">' +
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="size-6 text-yellow-600"><path d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" stroke-linecap="round" stroke-linejoin="round" /></svg></div>',
  danger: '<div class="mx-auto flex size-12 shrink-0 items-center justify-center rounded-full bg-red-100 sm:mx-0 sm:size-10">' +
    '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="size-6 text-red-600"><path d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126ZM12 15.75h.007v.008H12v-.008Z" stroke-linecap="round" stroke-linejoin="round" /></svg></div>',
};

function showConfirmDialog(title, message, confirmLabel, confirmClass, onConfirm) {
  var overlay = document.createElement('div');
  overlay.className = 'fixed inset-0 z-50 overflow-y-auto';
  var iconType = confirmClass && confirmClass.includes('red') ? 'danger' : 'warning';
  overlay.innerHTML =
    '<div class="fixed inset-0 bg-gray-500/75 transition-opacity"></div>' +
    '<div class="flex min-h-full items-end justify-center p-4 text-center sm:items-center sm:p-0">' +
      '<div class="relative transform overflow-hidden rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-lg">' +
        '<div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">' +
          '<div class="sm:flex sm:items-start">' +
            DIALOG_ICONS[iconType] +
            '<div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left">' +
              '<h3 class="text-base font-semibold text-gray-900">' + escapeHtml(title) + '</h3>' +
              '<div class="mt-2"><p class="text-sm text-gray-500">' + escapeHtml(message) + '</p></div>' +
            '</div>' +
          '</div>' +
        '</div>' +
        '<div class="bg-gray-50 px-4 py-3 sm:flex sm:flex-row-reverse sm:px-6">' +
          '<button type="button" id="dlg-confirm" class="inline-flex w-full justify-center rounded-md px-3 py-2 text-sm font-semibold text-white shadow-xs sm:ml-3 sm:w-auto ' + (confirmClass || 'bg-indigo-600 hover:bg-indigo-500') + '">' + escapeHtml(confirmLabel) + '</button>' +
          '<button type="button" id="dlg-cancel" class="mt-3 inline-flex w-full justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-xs ring-1 ring-inset ring-gray-300 hover:bg-gray-50 sm:mt-0 sm:w-auto">Cancel</button>' +
        '</div>' +
      '</div>' +
    '</div>';
  document.body.appendChild(overlay);
  document.getElementById('dlg-cancel').addEventListener('click', function() { overlay.remove(); });
  document.getElementById('dlg-confirm').addEventListener('click', function() { overlay.remove(); onConfirm(); });
}

function showPromptDialog(title, message, placeholder, confirmLabel, confirmClass, onConfirm) {
  var overlay = document.createElement('div');
  overlay.className = 'fixed inset-0 z-50 overflow-y-auto';
  var iconType = confirmClass && confirmClass.includes('red') ? 'danger' : 'warning';
  overlay.innerHTML =
    '<div class="fixed inset-0 bg-gray-500/75 transition-opacity"></div>' +
    '<div class="flex min-h-full items-end justify-center p-4 text-center sm:items-center sm:p-0">' +
      '<div class="relative transform overflow-hidden rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 sm:w-full sm:max-w-lg">' +
        '<div class="bg-white px-4 pt-5 pb-4 sm:p-6 sm:pb-4">' +
          '<div class="sm:flex sm:items-start">' +
            DIALOG_ICONS[iconType] +
            '<div class="mt-3 text-center sm:mt-0 sm:ml-4 sm:text-left w-full">' +
              '<h3 class="text-base font-semibold text-gray-900">' + escapeHtml(title) + '</h3>' +
              '<div class="mt-2"><p class="text-sm text-gray-500">' + escapeHtml(message) + '</p></div>' +
              '<input type="text" id="dlg-input" placeholder="' + escapeHtml(placeholder || '') + '" class="mt-3 block w-full rounded-md border-0 py-1.5 text-sm text-gray-900 shadow-xs ring-1 ring-inset ring-gray-300 placeholder:text-gray-400 focus:ring-2 focus:ring-inset focus:ring-indigo-600">' +
            '</div>' +
          '</div>' +
        '</div>' +
        '<div class="bg-gray-50 px-4 py-3 sm:flex sm:flex-row-reverse sm:px-6">' +
          '<button type="button" id="dlg-confirm" class="inline-flex w-full justify-center rounded-md px-3 py-2 text-sm font-semibold text-white shadow-xs sm:ml-3 sm:w-auto ' + (confirmClass || 'bg-indigo-600 hover:bg-indigo-500') + '">' + escapeHtml(confirmLabel) + '</button>' +
          '<button type="button" id="dlg-cancel" class="mt-3 inline-flex w-full justify-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-xs ring-1 ring-inset ring-gray-300 hover:bg-gray-50 sm:mt-0 sm:w-auto">Cancel</button>' +
        '</div>' +
      '</div>' +
    '</div>';
  document.body.appendChild(overlay);
  document.getElementById('dlg-cancel').addEventListener('click', function() { overlay.remove(); });
  document.getElementById('dlg-confirm').addEventListener('click', function() {
    var val = document.getElementById('dlg-input').value;
    overlay.remove();
    onConfirm(val);
  });
}

// --- Router ---

var router = {
  routes: [],

  add: function(pattern, handler) {
    this.routes.push({ pattern: pattern, handler: handler });
    return this;
  },

  navigate: function(url) {
    history.pushState(null, '', UI_BASE + url);
    this.resolve();
  },

  resolve: function() {
    cleanupSSE();
    var raw = location.pathname;
    var path = raw.replace(new RegExp('^' + UI_BASE + '/?'), '/').replace(/\/+$/, '') || '/';

    for (var i = 0; i < this.routes.length; i++) {
      var match = path.match(this.routes[i].pattern);
      if (match) {
        this.routes[i].handler(match);
        return;
      }
    }
    document.getElementById('app').innerHTML = '<div class="text-center py-12 text-gray-500">Page not found</div>';
  },

  init: function() {
    var self = this;
    window.addEventListener('popstate', function() { self.resolve(); });
    document.addEventListener('click', function(e) {
      var a = e.target.closest('a[data-link]');
      if (a) {
        e.preventDefault();
        self.navigate(a.getAttribute('href'));
      }
    });
    this.resolve();
  }
};

// --- Views ---

function renderLoading() {
  return '<div class="flex justify-center py-12">' +
    '<svg class="animate-spin h-8 w-8 text-indigo-600" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">' +
    '<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>' +
    '<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>' +
    '</svg></div>';
}

function renderError(msg) {
  return '<div class="rounded-md bg-red-50 p-4 mx-4"><div class="text-sm text-red-700">' + escapeHtml(msg) + '</div></div>';
}

// --- Workflow List ---

function buildWorkflowRows(workflows) {
  if (workflows.length === 0) {
    return '<tr><td colspan="7" class="py-8 text-center text-sm text-gray-500">No workflows found</td></tr>';
  }
  return workflows.map(function(wf) {
    var ns = wf.namespace && wf.namespace !== 'default' ? '?namespace=' + encodeURIComponent(wf.namespace) : '';
    var attrHtml = outcomeBadge(wf.searchAttributes);
    return '<tr>' +
      '<td class="border-b border-gray-200 py-4 pr-3 pl-4 text-sm whitespace-nowrap sm:pl-6 lg:pl-8"><span class="font-mono text-xs text-gray-900">' + escapeHtml(wf.id) + '</span></td>' +
      '<td class="hidden border-b border-gray-200 px-3 py-4 text-sm whitespace-nowrap text-gray-500 sm:table-cell">' + escapeHtml(wf.workflowType) + '</td>' +
      '<td class="border-b border-gray-200 px-3 py-4 text-sm whitespace-nowrap">' + statusBadge(wf.status) + '</td>' +
      '<td class="border-b border-gray-200 px-3 py-4 text-sm">' + (attrHtml || '<span class="text-gray-300">-</span>') + '</td>' +
      '<td class="hidden border-b border-gray-200 px-3 py-4 text-sm whitespace-nowrap text-gray-500 lg:table-cell">' + escapeHtml(wf.taskQueue) + '</td>' +
      '<td class="hidden border-b border-gray-200 px-3 py-4 text-sm whitespace-nowrap text-gray-500 sm:table-cell">' + formatTime(wf.createdAt) + '</td>' +
      '<td class="border-b border-gray-200 py-4 pr-4 pl-3 text-right text-sm font-medium whitespace-nowrap sm:pr-6 lg:pr-8">' +
        '<a href="/workflows/' + wf.id + ns + '" data-link class="text-indigo-600 hover:text-indigo-900">Detail</a>' +
      '</td>' +
    '</tr>';
  }).join('');
}

async function viewWorkflowList() {
  var app = document.getElementById('app');
  app.innerHTML = renderLoading();

  try {
    var params = new URLSearchParams(location.search);
    var namespace = params.get('namespace') || '';
    var statusFilter = params.get('status') || '';
    var outcomeFilter = params.get('outcome') || '';

    var pageToken = params.get('page') || '';
    var PAGE_SIZE = 15;

    var query = new URLSearchParams();
    query.set('page_size', String(PAGE_SIZE));
    if (namespace) query.set('namespace', namespace);
    if (statusFilter) query.set('status_filter', statusFilter);
    if (outcomeFilter) query.set('search_attributes_filter[outcome]', outcomeFilter);
    if (pageToken) query.set('next_page_token', pageToken);

    var data = await api('/workflows?' + query.toString());
    var workflows = data.workflows || [];
    var nextPageToken = data.nextPageToken || '';

    var rows = buildWorkflowRows(workflows);

    var statusOptions = Object.keys(STATUS_CONFIG).map(function(k) {
      var selected = statusFilter === k ? ' selected' : '';
      return '<option value="' + k + '"' + selected + '>' + STATUS_CONFIG[k].label + '</option>';
    }).join('');

    app.innerHTML =
      '<div class="px-4 sm:px-6 lg:px-8">' +
        '<div class="sm:flex sm:items-center">' +
          '<div class="sm:flex-auto">' +
            '<h1 class="text-base font-semibold text-gray-900">Workflows</h1>' +
            '<p class="mt-2 text-sm text-gray-700">Workflow executions</p>' +
          '</div>' +
        '</div>' +
        '<div class="mt-4 flex flex-wrap gap-4 items-end">' +
          '<div>' +
            '<label for="ns" class="block text-sm font-medium text-gray-700">Namespace</label>' +
            '<input type="text" id="ns" value="' + escapeHtml(namespace) + '" placeholder="default" class="mt-1 block w-40 rounded-md border border-gray-300 px-3 py-1.5 text-sm shadow-sm focus:border-indigo-500 focus:ring-indigo-500">' +
          '</div>' +
          '<div>' +
            '<label for="st" class="block text-sm font-medium text-gray-700">Status</label>' +
            '<select id="st" class="mt-1 block w-44 rounded-md border border-gray-300 px-3 py-1.5 text-sm shadow-sm focus:border-indigo-500 focus:ring-indigo-500">' +
              '<option value="">All</option>' +
              statusOptions +
            '</select>' +
          '</div>' +
          '<div>' +
            '<label for="oc" class="block text-sm font-medium text-gray-700">Outcome</label>' +
            '<select id="oc" class="mt-1 block w-52 rounded-md border border-gray-300 px-3 py-1.5 text-sm shadow-sm focus:border-indigo-500 focus:ring-indigo-500">' +
              '<option value="">All</option>' +
              Object.keys(OUTCOME_CONFIG).map(function(val) {
                var selected = outcomeFilter === val ? ' selected' : '';
                return '<option value="' + val + '"' + selected + '>' + OUTCOME_CONFIG[val].label + '</option>';
              }).join('') +
            '</select>' +
          '</div>' +
          '<button id="btn-filter" class="rounded-md bg-indigo-600 px-3 py-2 text-sm font-semibold text-white shadow-sm hover:bg-indigo-500">Filter</button>' +
        '</div>' +
        '<div class="mt-8 flow-root">' +
          '<div class="-mx-4 -my-2 overflow-x-auto sm:-mx-6 lg:-mx-8">' +
            '<div class="inline-block min-w-full py-2 align-middle">' +
              '<table id="workflow-table" class="min-w-full border-separate border-spacing-0">' +
                '<thead>' +
                  '<tr>' +
                    '<th scope="col" class="sticky top-0 z-10 border-b border-gray-300 bg-white/75 py-3.5 pr-3 pl-4 text-left text-sm font-semibold text-gray-900 backdrop-blur-sm sm:pl-6 lg:pl-8">Workflow ID</th>' +
                    '<th scope="col" class="sticky top-0 z-10 hidden border-b border-gray-300 bg-white/75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur-sm sm:table-cell">Type</th>' +
                    '<th scope="col" class="sticky top-0 z-10 border-b border-gray-300 bg-white/75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur-sm">Status</th>' +
                    '<th scope="col" class="sticky top-0 z-10 border-b border-gray-300 bg-white/75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur-sm">Outcome</th>' +
                    '<th scope="col" class="sticky top-0 z-10 hidden border-b border-gray-300 bg-white/75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur-sm lg:table-cell">Task Queue</th>' +
                    '<th scope="col" class="sticky top-0 z-10 hidden border-b border-gray-300 bg-white/75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur-sm sm:table-cell">Created</th>' +
                    '<th scope="col" class="sticky top-0 z-10 border-b border-gray-300 bg-white/75 py-3.5 pr-4 pl-3 backdrop-blur-sm sm:pr-6 lg:pr-8"><span class="sr-only">Detail</span></th>' +
                  '</tr>' +
                '</thead>' +
                '<tbody>' + rows + '</tbody>' +
              '</table>' +
            '</div>' +
          '</div>' +
        '</div>' +
        '<nav aria-label="Pagination" class="flex items-center justify-between border-t border-gray-200 bg-white px-4 py-3 sm:px-6">' +
          '<div class="hidden sm:block">' +
            '<p class="text-sm text-gray-700">Showing <span class="font-medium">' + (workflows.length === 0 ? '0' : String((pageToken ? '...' : '1'))) + '</span> to <span class="font-medium">' + workflows.length + '</span> results</p>' +
          '</div>' +
          '<div class="flex flex-1 justify-between sm:justify-end">' +
            (pageToken
              ? '<button id="btn-prev" class="relative inline-flex items-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50">Previous</button>'
              : '') +
            (nextPageToken
              ? '<button id="btn-next" class="relative ml-3 inline-flex items-center rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-700 ring-1 ring-inset ring-gray-300 hover:bg-gray-50">Next</button>'
              : '') +
          '</div>' +
        '</nav>' +
      '</div>';

    var btnFilter = document.getElementById('btn-filter');
    if (btnFilter) {
      btnFilter.addEventListener('click', function() {
        var p = new URLSearchParams();
        var nsVal = document.getElementById('ns').value.trim();
        var stVal = document.getElementById('st').value;
        var ocVal = document.getElementById('oc').value;
        if (nsVal) p.set('namespace', nsVal);
        if (stVal) p.set('status', stVal);
        if (ocVal) p.set('outcome', ocVal);
        var qs = p.toString();
        router.navigate('/' + (qs ? '?' + qs : ''));
      });
    }

    var btnNext = document.getElementById('btn-next');
    if (btnNext) {
      btnNext.addEventListener('click', function() {
        var p = new URLSearchParams(location.search);
        p.set('page', nextPageToken);
        router.navigate('/?' + p.toString());
      });
    }
    var btnPrev = document.getElementById('btn-prev');
    if (btnPrev) {
      btnPrev.addEventListener('click', function() {
        history.back();
      });
    }

    // SSE for live updates
    var sseUrl = '/v1/sse/workflows';
    if (namespace) sseUrl += '?namespace=' + encodeURIComponent(namespace);
    var queryStr = query.toString();
    currentEventSource = new EventSource(sseUrl);
    currentEventSource.onmessage = async function() {
      try {
        var freshData = await api('/workflows?' + queryStr);
        var freshWorkflows = freshData.workflows || [];
        var tbody = document.querySelector('#workflow-table tbody');
        if (tbody) {
          tbody.innerHTML = buildWorkflowRows(freshWorkflows);
        }
      } catch (e) {}
    };
  } catch (err) {
    app.innerHTML = renderError(err.message);
  }
}

// --- Workflow Detail ---

async function viewWorkflowDetail(match) {
  var app = document.getElementById('app');
  if (!app.querySelector('[data-workflow-detail]')) {
    app.innerHTML = renderLoading();
  }

  var workflowId = match[1];
  var params = new URLSearchParams(location.search);
  var namespace = params.get('namespace') || '';
  var nsQuery = namespace ? '?namespace=' + encodeURIComponent(namespace) : '';

  try {
    var results = await Promise.all([
      api('/workflows/' + workflowId + nsQuery),
      api('/workflows/' + workflowId + '/history' + nsQuery),
    ]);

    var wf = results[0].workflowExecution || {};
    var events = (results[1].events || []).sort(function(a, b) { return a.sequenceNum - b.sequenceNum; });

    var input = decodeBytes(wf.input);
    var result = decodeBytes(wf.result);

    // Breadcrumb
    var breadcrumb =
      '<nav aria-label="Breadcrumb" class="mb-6">' +
        '<ol role="list" class="flex items-center space-x-4">' +
          '<li><a href="/" data-link class="text-gray-400 hover:text-gray-500">' +
            '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 shrink-0"><path d="M9.293 2.293a1 1 0 011.414 0l7 7A1 1 0 0117 11h-1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-3a1 1 0 00-1-1H9a1 1 0 00-1 1v3a1 1 0 01-1 1H5a1 1 0 01-1-1v-6H3a1 1 0 01-.707-1.707l7-7z" clip-rule="evenodd" fill-rule="evenodd" /></svg>' +
            '<span class="sr-only">Home</span></a></li>' +
          '<li><div class="flex items-center">' +
            '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 shrink-0 text-gray-400"><path d="M8.22 5.22a.75.75 0 011.06 0l4.25 4.25a.75.75 0 010 1.06l-4.25 4.25a.75.75 0 01-1.06-1.06L11.94 10 8.22 6.28a.75.75 0 010-1.06z" clip-rule="evenodd" fill-rule="evenodd" /></svg>' +
            '<a href="/" data-link class="ml-4 text-sm font-medium text-gray-500 hover:text-gray-700">Workflows</a>' +
          '</div></li>' +
          '<li><div class="flex items-center">' +
            '<svg viewBox="0 0 20 20" fill="currentColor" class="size-5 shrink-0 text-gray-400"><path d="M8.22 5.22a.75.75 0 011.06 0l4.25 4.25a.75.75 0 010 1.06l-4.25 4.25a.75.75 0 01-1.06-1.06L11.94 10 8.22 6.28a.75.75 0 010-1.06z" clip-rule="evenodd" fill-rule="evenodd" /></svg>' +
            '<span class="ml-4 text-sm font-medium text-gray-500">' + escapeHtml(wf.id) + '</span>' +
          '</div></li>' +
        '</ol>' +
      '</nav>';

    // Description list rows
    var dlRows =
      dlRow('Workflow ID', '<span class="font-mono">' + escapeHtml(wf.id) + '</span>') +
      dlRow('Status', statusBadge(wf.status)) +
      dlRow('Type', escapeHtml(wf.workflowType)) +
      dlRow('Namespace', escapeHtml(wf.namespace || 'default')) +
      dlRow('Task Queue', escapeHtml(wf.taskQueue)) +
      dlRow('Created', formatTime(wf.createdAt));

    if (isValidTime(wf.closedAt)) {
      dlRows += dlRow('Closed', formatTime(wf.closedAt));
    }
    if (wf.errorMessage) {
      dlRows += dlRow('Error', '<span class="text-red-700">' + escapeHtml(wf.errorMessage) + '</span>');
    }
    if (wf.searchAttributes && wf.searchAttributes.outcome) {
      dlRows += dlRow('Outcome', outcomeBadge(wf.searchAttributes));
    }
    if (wf.parentWorkflowId) {
      dlRows += dlRow('Parent Workflow', '<a href="/workflows/' + wf.parentWorkflowId + nsQuery + '" data-link class="text-indigo-600 hover:text-indigo-900 font-mono">' + escapeHtml(wf.parentWorkflowId) + '</a>');
    }
    if (wf.continuedAsNewId) {
      dlRows += dlRow('Continued As', '<a href="/workflows/' + wf.continuedAsNewId + nsQuery + '" data-link class="text-indigo-600 hover:text-indigo-900 font-mono">' + escapeHtml(wf.continuedAsNewId) + '</a>');
    }
    if (wf.cronSchedule) {
      dlRows += dlRow('Cron Schedule', '<span class="font-mono">' + escapeHtml(wf.cronSchedule) + '</span>');
    }
    if (input !== null) {
      dlRows += dlRow('Input', '<pre class="bg-gray-50 rounded-md p-2 overflow-x-auto text-xs">' + formatJson(input) + '</pre>');
    }
    if (result !== null) {
      dlRows += dlRow('Result', '<pre class="bg-gray-50 rounded-md p-2 overflow-x-auto text-xs">' + formatJson(result) + '</pre>');
    }

    // Actions for running workflows
    var isRunning = wf.status === 'WORKFLOW_EXECUTION_STATUS_RUNNING';
    var actionsHtml = '';
    if (isRunning) {
      actionsHtml =
        '<div class="mt-4 rounded-lg border border-gray-200 bg-gray-50 p-4">' +
          '<h4 class="text-sm font-semibold text-gray-900 mb-3">Actions</h4>' +
          '<div class="space-y-3">' +
            '<div>' +
              '<p class="text-xs text-gray-500 mb-2">Send a signal to the workflow</p>' +
              '<div class="flex gap-2 items-end">' +
                '<div>' +
                  '<label for="sig-name" class="block text-xs text-gray-600">Signal Name</label>' +
                  '<input type="text" id="sig-name" placeholder="human-decision" class="mt-0.5 block w-40 rounded-md border border-gray-300 px-2 py-1.5 text-sm shadow-sm focus:border-indigo-500 focus:ring-indigo-500">' +
                '</div>' +
                '<div class="flex-1">' +
                  '<label for="sig-input" class="block text-xs text-gray-600">Input (JSON)</label>' +
                  '<input type="text" id="sig-input" placeholder=\'{"action":"retry"}\' class="mt-0.5 block w-full rounded-md border border-gray-300 px-2 py-1.5 text-sm font-mono shadow-sm focus:border-indigo-500 focus:ring-indigo-500">' +
                '</div>' +
                '<button id="btn-signal" class="rounded-md bg-indigo-600 px-3 py-1.5 text-sm font-semibold text-white shadow-sm hover:bg-indigo-500">Send Signal</button>' +
              '</div>' +
            '</div>' +
            '<div class="flex gap-3 border-t border-gray-200 pt-3">' +
              '<button id="btn-cancel" class="rounded-md bg-yellow-500 px-3 py-1.5 text-sm font-semibold text-white shadow-sm hover:bg-yellow-400">Cancel Workflow</button>' +
              '<button id="btn-terminate" class="rounded-md bg-red-600 px-3 py-1.5 text-sm font-semibold text-white shadow-sm hover:bg-red-500">Terminate Workflow</button>' +
            '</div>' +
          '</div>' +
        '</div>';
    }

    var descriptionList =
      '<div class="overflow-hidden bg-white shadow-sm sm:rounded-lg">' +
        '<div class="px-4 py-6 sm:px-6">' +
          '<h3 class="text-base font-semibold text-gray-900">Workflow Information</h3>' +
          '<p class="mt-1 max-w-2xl text-sm text-gray-500">' + escapeHtml(wf.workflowType) + '</p>' +
        '</div>' +
        '<div class="border-t border-gray-100">' +
          '<dl class="divide-y divide-gray-100">' + dlRows + '</dl>' +
        '</div>' +
        actionsHtml +
      '</div>';

    // Event timeline
    var timeline;
    if (events.length === 0) {
      timeline = '<p class="text-sm text-gray-500 py-4">No history events</p>';
    } else {
      var items = events.map(function(evt, i) {
        var isLast = i === events.length - 1;
        var color = EVENT_COLORS[evt.eventType] || 'bg-gray-400';
        var evtData = decodeBytes(evt.eventData);

        var dataHtml = '';
        if (evtData) {
          dataHtml = '<details class="mt-1"><summary class="text-xs text-gray-400 cursor-pointer hover:text-gray-600">Event Data</summary>' +
            '<pre class="mt-1 bg-gray-50 rounded-md p-2 text-xs overflow-x-auto">' + formatJson(evtData) + '</pre></details>';
        }

        return '<li>' +
          '<div class="relative ' + (isLast ? '' : 'pb-8') + '">' +
            (isLast ? '' : '<span aria-hidden="true" class="absolute top-4 left-4 -ml-px h-full w-0.5 bg-gray-200"></span>') +
            '<div class="relative flex space-x-3">' +
              '<div><span class="flex size-8 items-center justify-center rounded-full ' + color + ' ring-8 ring-white">' +
                eventIcon(evt.eventType) +
              '</span></div>' +
              '<div class="flex min-w-0 flex-1 justify-between space-x-4 pt-1.5">' +
                '<div>' +
                  '<p class="text-sm text-gray-500"><span class="font-medium text-gray-900">#' + evt.sequenceNum + '</span> ' + escapeHtml(evt.eventType) + '</p>' +
                  dataHtml +
                '</div>' +
                '<div class="text-right text-sm whitespace-nowrap text-gray-500"><time>' + formatTime(evt.timestamp) + '</time></div>' +
              '</div>' +
            '</div>' +
          '</div>' +
        '</li>';
      }).join('');

      timeline = '<div class="flow-root"><ul role="list" class="-mb-8">' + items + '</ul></div>';
    }

    app.innerHTML =
      '<div data-workflow-detail class="px-4 sm:px-6 lg:px-8">' +
        breadcrumb +
        descriptionList +
        '<div class="mt-8">' +
          '<h3 class="text-base font-semibold text-gray-900 mb-4">History</h3>' +
          timeline +
        '</div>' +
      '</div>';

    // Bind action buttons
    var btnSignal = document.getElementById('btn-signal');
    if (btnSignal) {
      btnSignal.addEventListener('click', async function() {
        var sigName = document.getElementById('sig-name').value.trim();
        if (!sigName) { showToast('Signal name is required', 'warning'); return; }
        var sigInputRaw = document.getElementById('sig-input').value.trim() || '{}';
        try { JSON.parse(sigInputRaw); } catch (e) { showToast('Invalid JSON: ' + e.message, 'error'); return; }
        try {
          await apiPost('/workflows/' + workflowId + '/signals', {
            signalName: sigName,
            input: btoa(sigInputRaw),
            namespace: namespace || 'default',
          });
          showToast('Signal "' + sigName + '" sent successfully', 'success');
          viewWorkflowDetail(match);
        } catch (e) {
          showToast('Signal failed: ' + e.message, 'error');
        }
      });
    }
    var btnCancel = document.getElementById('btn-cancel');
    if (btnCancel) {
      btnCancel.addEventListener('click', function() {
        showConfirmDialog(
          'Cancel Workflow',
          'The workflow will receive a cancellation request and can run compensation logic before stopping.',
          'Cancel Workflow',
          'bg-yellow-500 hover:bg-yellow-400',
          async function() {
            try {
              await apiPost('/workflows/' + workflowId + '/cancellation', { namespace: namespace || 'default' });
              showToast('Cancellation request sent', 'success');
              viewWorkflowDetail(match);
            } catch (e) {
              showToast('Cancel failed: ' + e.message, 'error');
            }
          }
        );
      });
    }
    var btnTerminate = document.getElementById('btn-terminate');
    if (btnTerminate) {
      btnTerminate.addEventListener('click', function() {
        showPromptDialog(
          'Terminate Workflow',
          'This will immediately stop the workflow. This action cannot be undone.',
          'Reason (optional)',
          'Terminate',
          'bg-red-600 hover:bg-red-500',
          async function(reason) {
            try {
              await apiPost('/workflows/' + workflowId + '/termination', { reason: reason, namespace: namespace || 'default' });
              showToast('Workflow terminated', 'success');
              viewWorkflowDetail(match);
            } catch (e) {
              showToast('Terminate failed: ' + e.message, 'error');
            }
          }
        );
      });
    }

    // SSE for live updates on this workflow
    if (!currentEventSource) {
      var sseUrl = '/v1/sse/workflows';
      if (namespace) sseUrl += '?namespace=' + encodeURIComponent(namespace);
      currentEventSource = new EventSource(sseUrl);
      currentEventSource.onmessage = function(e) {
        try {
          var notification = JSON.parse(e.data);
          if (notification.workflowId === workflowId) {
            viewWorkflowDetail(match);
          }
        } catch (ex) {}
      };
    }

  } catch (err) {
    app.innerHTML = renderError(err.message);
  }
}

function dlRow(label, value) {
  return '<div class="px-4 py-4 sm:grid sm:grid-cols-3 sm:gap-4 sm:px-6">' +
    '<dt class="text-sm font-medium text-gray-900">' + label + '</dt>' +
    '<dd class="mt-1 text-sm text-gray-700 sm:col-span-2 sm:mt-0">' + value + '</dd>' +
  '</div>';
}

// --- SSE ---

var currentEventSource = null;

function cleanupSSE() {
  if (currentEventSource) {
    currentEventSource.close();
    currentEventSource = null;
  }
}

// --- Init ---

router
  .add(/^\/workflows\/(.+)$/, viewWorkflowDetail)
  .add(/^\/?(\?.*)?$/, viewWorkflowList);

document.addEventListener('DOMContentLoaded', function() { router.init(); });
