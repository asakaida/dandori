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
  if (eventType.includes('Signal')) {
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
    return '<tr><td colspan="6" class="py-8 text-center text-sm text-gray-500">No workflows found</td></tr>';
  }
  return workflows.map(function(wf) {
    var ns = wf.namespace && wf.namespace !== 'default' ? '?namespace=' + encodeURIComponent(wf.namespace) : '';
    return '<tr>' +
      '<td class="border-b border-gray-200 py-4 pr-3 pl-4 text-sm whitespace-nowrap sm:pl-6 lg:pl-8"><span class="font-mono text-xs text-gray-900">' + escapeHtml(wf.id) + '</span></td>' +
      '<td class="hidden border-b border-gray-200 px-3 py-4 text-sm whitespace-nowrap text-gray-500 sm:table-cell">' + escapeHtml(wf.workflowType) + '</td>' +
      '<td class="border-b border-gray-200 px-3 py-4 text-sm whitespace-nowrap">' + statusBadge(wf.status) + '</td>' +
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

    var query = new URLSearchParams();
    query.set('page_size', '100');
    if (namespace) query.set('namespace', namespace);
    if (statusFilter) query.set('status_filter', statusFilter);

    var data = await api('/workflows?' + query.toString());
    var workflows = data.workflows || [];

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
      '</div>';

    var btnFilter = document.getElementById('btn-filter');
    if (btnFilter) {
      btnFilter.addEventListener('click', function() {
        var p = new URLSearchParams();
        var nsVal = document.getElementById('ns').value.trim();
        var stVal = document.getElementById('st').value;
        if (nsVal) p.set('namespace', nsVal);
        if (stVal) p.set('status', stVal);
        var qs = p.toString();
        router.navigate('/' + (qs ? '?' + qs : ''));
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

    var descriptionList =
      '<div class="overflow-hidden bg-white shadow-sm sm:rounded-lg">' +
        '<div class="px-4 py-6 sm:px-6">' +
          '<h3 class="text-base font-semibold text-gray-900">Workflow Information</h3>' +
          '<p class="mt-1 max-w-2xl text-sm text-gray-500">' + escapeHtml(wf.workflowType) + '</p>' +
        '</div>' +
        '<div class="border-t border-gray-100">' +
          '<dl class="divide-y divide-gray-100">' + dlRows + '</dl>' +
        '</div>' +
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
