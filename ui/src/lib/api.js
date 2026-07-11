// AgentNativeDB HTTP API 客户端

const BASE = './api/v1';

async function request(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  };
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(BASE + path, opts);
  const data = await res.json();
  if (!data.ok) {
    throw new Error(data.error || '未知错误');
  }
  return data.data;
}

// ========== 健康检查 ==========

export async function healthCheck() {
  const res = await fetch('./health');
  return await res.json();
}

// ========== Session ==========

export async function listSessions(agentId, limit) {
  const params = new URLSearchParams();
  if (agentId) params.set('agent_id', agentId);
  if (limit) params.set('limit', String(limit));
  const qs = params.toString();
  return request('GET', '/sessions' + (qs ? '?' + qs : ''));
}

export async function getSession(id) {
  return request('GET', `/sessions/${id}`);
}

export async function createSession(agentId, metadata) {
  return request('POST', '/sessions', { agent_id: agentId, metadata });
}

export async function updateSession(id, state, context) {
  return request('PATCH', `/sessions/${id}`, { state, context });
}

export async function deleteSession(id) {
  return request('DELETE', `/sessions/${id}`);
}

// ========== Memory ==========

export async function listMemories(sessionId, type, limit) {
  const params = new URLSearchParams({ session_id: sessionId });
  if (type) params.set('type', type);
  if (limit) params.set('limit', String(limit));
  return request('GET', '/memories?' + params.toString());
}

export async function getMemory(id) {
  return request('GET', `/memories/${id}`);
}

export async function storeMemory(sessionId, type, content, importance) {
  return request('POST', '/memories', {
    session_id: sessionId,
    type,
    content,
    importance,
  });
}

export async function deleteMemory(id) {
  return request('DELETE', `/memories/${id}`);
}

// ========== Decision ==========

export async function listDecisions(sessionId, limit) {
  const params = new URLSearchParams({ session_id: sessionId });
  if (limit) params.set('limit', String(limit));
  return request('GET', '/decisions?' + params.toString());
}

export async function getDecision(id) {
  return request('GET', `/decisions/${id}`);
}

export async function recordDecision(d) {
  return request('POST', '/decisions', d);
}

export async function deleteDecision(id) {
  return request('DELETE', `/decisions/${id}`);
}

export async function getDecisionTree(id) {
  return request('GET', `/decisions/${id}/tree`);
}

// ========== SQL ==========

export async function executeQuery(sql) {
  return request('POST', '/query', { sql });
}

// ========== Vector ==========

export async function listVectorIndexes() {
  return request('GET', '/vector/indexes');
}

export async function createVectorIndex(name, dim, metric) {
  return request('POST', '/vector/indexes', { name, dim, metric });
}

export async function getVectorIndex(name) {
  return request('GET', `/vector/indexes/${encodeURIComponent(name)}`);
}

export async function insertVector(name, id, vector, payload) {
  const body = { id, vector };
  if (payload !== undefined && payload !== null) {
    body.payload = payload;
  }
  return request('POST', `/vector/indexes/${encodeURIComponent(name)}/vectors`, body);
}

export async function deleteVector(name, id) {
  return request('DELETE', `/vector/indexes/${encodeURIComponent(name)}/vectors/${encodeURIComponent(id)}`);
}

export async function searchVector(name, vector, topK) {
  return request('POST', `/vector/indexes/${encodeURIComponent(name)}/search`, { vector, top_k: topK });
}

// ========== Cluster ==========

export async function getClusterStatus() {
  return request('GET', '/cluster/status');
}

export async function addPeer(nodeId, raftAddr) {
  return request('POST', '/cluster/peers', { node_id: nodeId, raft_addr: raftAddr });
}

export async function removePeer(nodeId) {
  return request('DELETE', `/cluster/peers/${encodeURIComponent(nodeId)}`);
}
