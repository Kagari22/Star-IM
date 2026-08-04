<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'

const token = ref(localStorage.getItem('token') || '')
const me = ref(readStoredUser())
const authMode = ref('login')
const authForm = ref({ username: '', nickname: '', password: '' })
const users = ref([])
const selectedPeer = ref(null)
const messages = ref([])
const searchQuery = ref('')
const searchResults = ref([])
const composer = ref('')
const selectedFile = ref(null)
const fileInput = ref(null)
const socket = ref(null)
const socketState = ref('idle')
const loading = ref(false)
const syncing = ref(false)
const toast = ref('')
const toastTimer = ref(null)
const messageBox = ref(null)
let reconnectTimer = null

const isAuthenticated = computed(() => Boolean(token.value && me.value))
const currentTitle = computed(() => selectedPeer.value ? selectedPeer.value.nickname || selectedPeer.value.username : '选择一位成员开始对话')
const currentSubtitle = computed(() => selectedPeer.value ? '安全直连 · 实时同步中' : '从左侧成员列表开启一次对话')
const socketLabel = computed(() => {
  if (socketState.value === 'open') return '实时连接'
  if (socketState.value === 'connecting') return '正在连线'
  return '离线重连中'
})

function readStoredUser() {
  try {
    return JSON.parse(localStorage.getItem('me') || 'null')
  } catch {
    return null
  }
}

function showToast(message) {
  toast.value = message
  window.clearTimeout(toastTimer.value)
  toastTimer.value = window.setTimeout(() => {
    toast.value = ''
  }, 3400)
}

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {})
  if (token.value) headers.set('Authorization', 'Bearer ' + token.value)
  if (options.body && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(path, { ...options, headers })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) {
    const error = new Error(data.error || '请求未能完成')
    error.status = response.status
    throw error
  }
  return data
}

async function submitAuth() {
  if (!authForm.value.username || !authForm.value.password) {
    showToast('请填写用户名和密码')
    return
  }

  loading.value = true
  try {
    if (authMode.value === 'register') {
      await api('/api/register', {
        method: 'POST',
        body: JSON.stringify(authForm.value)
      })
      authMode.value = 'login'
      authForm.value.password = ''
      showToast('注册成功，现在登录吧')
      return
    }

    const data = await api('/api/login', {
      method: 'POST',
      body: JSON.stringify({
        username: authForm.value.username,
        password: authForm.value.password
      })
    })
    token.value = data.token
    me.value = data.user
    localStorage.setItem('token', data.token)
    localStorage.setItem('me', JSON.stringify(data.user))
    await bootstrapWorkspace()
  } catch (error) {
    showToast(error.message)
  } finally {
    loading.value = false
  }
}

async function bootstrapWorkspace() {
  connectSocket()
  try {
    await Promise.all([refreshUsers(), loadOfflineMessages()])
  } catch (error) {
    if (error.status === 401) {
      clearSession()
      showToast('登录状态已失效，请重新登录')
      return
    }
    showToast(error.message)
  }
}

async function refreshUsers() {
  const data = await api('/api/users')
  users.value = data.users || []
}

async function selectPeer(user) {
  selectedPeer.value = user
  searchResults.value = []
  await loadConversation()
  await refreshUsers()
}

async function loadConversation() {
  if (!selectedPeer.value) return
  try {
    const data = await api('/api/messages?peer_id=' + selectedPeer.value.id + '&limit=100')
    messages.value = data.messages || []
    await scrollMessages()
  } catch (error) {
    showToast(error.message)
  }
}

function offlineCursorKey() {
  return me.value ? 'offline_cursor:' + me.value.id : ''
}

async function loadOfflineMessages() {
  if (!isAuthenticated.value) return

  syncing.value = true
  const key = offlineCursorKey()
  let afterID = Number.parseInt(localStorage.getItem(key) || '0', 10)
  if (!Number.isFinite(afterID) || afterID < 0) afterID = 0
  let count = 0

  try {
    while (true) {
      const data = await api('/api/offline?after_id=' + afterID + '&limit=100')
      const batch = data.messages || []
      if (batch.length === 0) break
      afterID = batch[batch.length - 1].id
      localStorage.setItem(key, String(afterID))
      count += batch.length
      if (batch.length < 100) break
    }
    if (count > 0) {
      showToast('已同步 ' + count + ' 条离线消息')
      await refreshUsers()
    }
  } finally {
    syncing.value = false
  }
}

function connectSocket() {
  if (!token.value) return
  if (socket.value) {
    socket.value.onclose = null
    socket.value.close()
  }
  window.clearTimeout(reconnectTimer)

  socketState.value = 'connecting'
  const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws'
  const url = scheme + '://' + window.location.host + '/ws'
  const connection = new WebSocket(url, ['im-chat', token.value])
  socket.value = connection

  connection.onopen = () => {
    socketState.value = 'open'
  }

  connection.onmessage = async (event) => {
    const payload = JSON.parse(event.data)
    if (payload.type === 'chat' || payload.type === 'ack') {
      appendMessage(payload.message)
      if (payload.type === 'chat') await refreshUsers()
      return
    }
    if (payload.type === 'error') showToast(payload.error)
  }

  connection.onerror = () => {
    socketState.value = 'closed'
  }

  connection.onclose = () => {
    socketState.value = 'closed'
    if (token.value) {
      reconnectTimer = window.setTimeout(connectSocket, 1800)
    }
  }
}

function appendMessage(message) {
  if (!message || !selectedPeer.value) return
  const inCurrentConversation =
    (message.from_id === me.value.id && message.to_id === selectedPeer.value.id) ||
    (message.from_id === selectedPeer.value.id && message.to_id === me.value.id)

  if (!inCurrentConversation || messages.value.some((item) => item.id === message.id)) return
  messages.value.push(message)
  messages.value.sort((left, right) => left.id - right.id)
  scrollMessages()
}

function sendMessage() {
  const content = composer.value.trim()
  if (!selectedPeer.value) {
    showToast('请先选择一位成员')
    return
  }
  if (!content) return
  if (!socket.value || socket.value.readyState !== WebSocket.OPEN) {
    showToast('实时连接尚未就绪')
    return
  }

  socket.value.send(JSON.stringify({
    type: 'chat',
    to: selectedPeer.value.id,
    content
  }))
  composer.value = ''
}

async function uploadMedia() {
  if (!selectedPeer.value) {
    showToast('请先选择一位成员')
    return
  }
  if (!selectedFile.value) {
    showToast('请先选择文件')
    return
  }

  const form = new FormData()
  form.append('file', selectedFile.value)
  form.append('to_user_id', String(selectedPeer.value.id))

  loading.value = true
  try {
    const data = await api('/api/media/upload', {
      method: 'POST',
      body: form
    })
    appendMessage(data.message)
    showToast('文件已发送')
    selectedFile.value = null
    if (fileInput.value) fileInput.value.value = ''
  } catch (error) {
    showToast(error.message)
  } finally {
    loading.value = false
  }
}

function pickFile(event) {
  selectedFile.value = event.target.files && event.target.files[0] ? event.target.files[0] : null
}

async function runSearch() {
  const query = searchQuery.value.trim()
  if (!query) {
    searchResults.value = []
    return
  }
  try {
    const peerSuffix = selectedPeer.value ? '&peer_id=' + selectedPeer.value.id : ''
    const data = await api('/api/search/messages?q=' + encodeURIComponent(query) + peerSuffix + '&limit=50')
    searchResults.value = data.messages || []
  } catch (error) {
    showToast(error.message)
  }
}

async function openSearchResult(message) {
  const peerID = message.from_id === me.value.id ? message.to_id : message.from_id
  const peer = users.value.find((user) => user.id === peerID)
  if (peer) {
    await selectPeer(peer)
  }
}

async function signOut() {
  try {
    await api('/api/logout', { method: 'POST' })
  } catch {
    // 网络不可用时仍允许用户清理本机登录状态。
  }
  clearSession()
}

function clearSession() {
  window.clearTimeout(reconnectTimer)
  if (socket.value) {
    socket.value.onclose = null
    socket.value.close()
  }
  socket.value = null
  token.value = ''
  me.value = null
  users.value = []
  selectedPeer.value = null
  messages.value = []
  searchResults.value = []
  localStorage.removeItem('token')
  localStorage.removeItem('me')
}

function avatarText(user) {
  const label = user && (user.nickname || user.username) ? user.nickname || user.username : '?'
  return Array.from(label).slice(0, 1).join('').toUpperCase()
}

function avatarStyle(user) {
  const label = user && (user.nickname || user.username) ? user.nickname || user.username : '?'
  let hash = 0
  for (const character of label) hash = (hash * 31 + character.codePointAt(0)) % 360
  return { '--avatar-hue': hash }
}

function isMine(message) {
  return me.value && message.from_id === me.value.id
}

function isImage(message) {
  return message.content_type === 'image' && message.object_url
}

function isFile(message) {
  return (message.content_type === 'file' || message.content_type === 'image') && message.object_url
}

function formatTime(value) {
  return new Date(value).toLocaleString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    month: 'numeric',
    day: 'numeric'
  })
}

function formatSize(size) {
  if (!size) return ''
  if (size < 1024 * 1024) return Math.ceil(size / 1024) + ' KB'
  return (size / 1024 / 1024).toFixed(1) + ' MB'
}

async function scrollMessages() {
  await nextTick()
  if (messageBox.value) messageBox.value.scrollTop = messageBox.value.scrollHeight
}

onMounted(() => {
  if (isAuthenticated.value) bootstrapWorkspace()
})

onBeforeUnmount(() => {
  window.clearTimeout(reconnectTimer)
  if (socket.value) socket.value.close()
})
</script>

<template>
  <main class="app-shell">
    <div class="aurora aurora-one"></div>
    <div class="aurora aurora-two"></div>
    <div class="star-field"></div>

    <Transition name="toast">
      <div v-if="toast" class="toast-message">
        <span class="toast-star">✦</span>
        {{ toast }}
      </div>
    </Transition>

    <section v-if="!isAuthenticated" class="welcome-page">
      <div class="welcome-visual">
        <div class="orbit orbit-large"></div>
        <div class="orbit orbit-small"></div>
        <div class="planet-core">
          <span>✦</span>
        </div>
        <div class="welcome-copy">
          <p class="eyebrow">TEAM SIGNAL / 01</p>
          <h1 class="hero-brand">
            <span>星</span><em>讯</em>
          </h1>
          <p class="welcome-description">
            为团队每一次灵感、确认与回应，留下有温度的实时轨迹。
          </p>
          <div class="feature-pills">
            <span>实时同步</span>
            <span>文件传递</span>
            <span>安全连接</span>
          </div>
        </div>
      </div>

      <form class="auth-card" @submit.prevent="submitAuth">
        <div class="auth-card-head">
          <div>
            <p class="eyebrow">WELCOME ABOARD</p>
            <h2>{{ authMode === 'login' ? '回到星图之中' : '创建新的坐标' }}</h2>
          </div>
          <div class="mini-emblem">✦</div>
        </div>

        <div class="auth-tabs" role="tablist">
          <button type="button" :class="{ active: authMode === 'login' }" @click="authMode = 'login'">登录</button>
          <button type="button" :class="{ active: authMode === 'register' }" @click="authMode = 'register'">注册</button>
        </div>

        <label class="field-label">
          <span>用户名</span>
          <input v-model.trim="authForm.username" autocomplete="username" placeholder="输入你的团队代号" />
        </label>

        <label v-if="authMode === 'register'" class="field-label">
          <span>昵称</span>
          <input v-model.trim="authForm.nickname" placeholder="显示给团队的名字" />
        </label>

        <label class="field-label">
          <span>密码</span>
          <input v-model="authForm.password" type="password" autocomplete="current-password" placeholder="至少 6 个字符" />
        </label>

        <button class="primary-action" type="submit" :disabled="loading">
          <span>{{ loading ? '正在连接星图…' : authMode === 'login' ? '进入工作台' : '创建账号' }}</span>
          <b>→</b>
        </button>

        <p class="auth-note">登录即表示以安全方式接入团队实时空间。</p>
      </form>
    </section>

    <section v-else class="workspace">
      <aside class="left-panel">
        <div class="brand-lockup">
          <div class="brand-star">✦</div>
          <div>
            <strong>星讯</strong>
            <span>TEAM CHAT</span>
          </div>
        </div>

        <div class="identity-card">
          <div class="avatar avatar-large" :style="avatarStyle(me)">{{ avatarText(me) }}</div>
          <div class="identity-copy">
            <strong>{{ me.nickname || me.username }}</strong>
            <span>@{{ me.username }}</span>
          </div>
          <button class="icon-button" aria-label="退出登录" title="退出登录" @click="signOut">↗</button>
        </div>

        <div class="member-heading">
          <div>
            <p>TEAM MEMBERS</p>
            <strong>团队成员</strong>
          </div>
          <button class="refresh-button" :class="{ spinning: syncing }" aria-label="刷新成员" @click="refreshUsers">↻</button>
        </div>

        <div class="member-list">
          <button
            v-for="user in users"
            :key="user.id"
            class="member-item"
            :class="{ active: selectedPeer && selectedPeer.id === user.id }"
            @click="selectPeer(user)"
          >
            <div class="avatar" :style="avatarStyle(user)">{{ avatarText(user) }}</div>
            <span class="member-name">{{ user.nickname || user.username }}</span>
            <b v-if="user.unread_count" class="unread-badge">{{ user.unread_count > 99 ? '99+' : user.unread_count }}</b>
          </button>
          <div v-if="!users.length" class="empty-members">还没有其他成员加入。</div>
        </div>

        <div class="connection-chip">
          <span :class="{ online: socketState === 'open' }"></span>
          {{ socketLabel }}
        </div>
      </aside>

      <section class="chat-panel">
        <header class="chat-header">
          <div class="chat-person">
            <template v-if="selectedPeer">
              <div class="avatar avatar-header" :style="avatarStyle(selectedPeer)">{{ avatarText(selectedPeer) }}</div>
              <div>
                <p>{{ currentSubtitle }}</p>
                <h2>{{ currentTitle }}</h2>
              </div>
            </template>
            <template v-else>
              <div class="compass-mark">✧</div>
              <div>
                <p>{{ currentSubtitle }}</p>
                <h2>{{ currentTitle }}</h2>
              </div>
            </template>
          </div>
          <div class="header-streak">
            <span></span><span></span><span></span>
          </div>
        </header>

        <div ref="messageBox" class="message-stage">
          <div v-if="!selectedPeer" class="empty-stage">
            <div class="empty-orb">✦</div>
            <p>从左侧选择一位成员</p>
            <span>聊天记录、搜索和文件都将在这里展开。</span>
          </div>

          <template v-else>
            <div class="conversation-marker">
              <span></span>
              <p>与 {{ selectedPeer.nickname || selectedPeer.username }} 的消息记录</p>
              <span></span>
            </div>

            <article
              v-for="message in messages"
              :key="message.id"
              class="message-row"
              :class="{ mine: isMine(message) }"
            >
              <div v-if="!isMine(message)" class="avatar message-avatar" :style="avatarStyle(selectedPeer)">
                {{ avatarText(selectedPeer) }}
              </div>
              <div class="message-content">
                <div class="message-meta">
                  <span>{{ isMine(message) ? '我' : selectedPeer.nickname || selectedPeer.username }}</span>
                  <time>{{ formatTime(message.created_at) }}</time>
                </div>
                <div class="message-bubble">
                  <img v-if="isImage(message)" :src="message.object_url" :alt="message.file_name || '图片消息'" />
                  <a v-else-if="isFile(message)" class="file-card" :href="message.object_url" target="_blank" rel="noreferrer">
                    <span class="file-icon">✧</span>
                    <span>
                      <b>{{ message.file_name || '附件' }}</b>
                      <small>{{ formatSize(message.file_size) || '点击打开文件' }}</small>
                    </span>
                    <i>↗</i>
                  </a>
                  <p v-else>{{ message.content }}</p>
                </div>
              </div>
            </article>
          </template>
        </div>

        <form class="composer" @submit.prevent="sendMessage">
          <div v-if="selectedFile" class="file-preview">
            <span>✧ {{ selectedFile.name }}</span>
            <button type="button" @click="selectedFile = null; fileInput.value = ''">×</button>
          </div>
          <textarea
            v-model="composer"
            :disabled="!selectedPeer"
            placeholder="写下想对团队伙伴说的话…"
            rows="1"
            @keydown.enter.exact.prevent="sendMessage"
          ></textarea>
          <div class="composer-actions">
            <label class="attach-button" title="上传文件">
              <input ref="fileInput" type="file" accept="image/jpeg,image/png,image/gif,image/webp,application/pdf,text/plain" @change="pickFile" />
              <span>＋ 附件</span>
            </label>
            <button v-if="selectedFile" type="button" class="send-file-button" :disabled="loading" @click="uploadMedia">发送文件</button>
            <button type="submit" class="send-button" :disabled="!selectedPeer || !composer.trim()">发送 <b>↗</b></button>
          </div>
        </form>
      </section>

      <aside class="right-panel">
        <div class="right-title">
          <p>LOOKING GLASS</p>
          <h3>消息回声</h3>
        </div>

        <form class="search-box" @submit.prevent="runSearch">
          <input v-model.trim="searchQuery" placeholder="搜索消息关键词" maxlength="64" />
          <button type="submit" aria-label="搜索">⌕</button>
        </form>

        <div class="search-context">
          <span class="spark">✦</span>
          <p>{{ selectedPeer ? '正在搜索当前会话' : '搜索你可查看的全部消息' }}</p>
        </div>

        <div class="result-list">
          <button v-for="result in searchResults" :key="result.id" class="result-card" @click="openSearchResult(result)">
            <p>{{ result.content || result.file_name || '媒体消息' }}</p>
            <span>#{{ result.id }} · {{ formatTime(result.created_at) }}</span>
          </button>
          <div v-if="searchQuery && !searchResults.length" class="search-empty">输入关键词后，在这里查看匹配消息。</div>
        </div>

        <div class="right-divider"></div>

        <div class="signal-card">
          <div class="signal-title">
            <span class="signal-dot"></span>
            <strong>连接状态</strong>
          </div>
          <p>{{ socketState === 'open' ? '你已接入团队实时消息网络。' : '正在尝试恢复实时消息连接。' }}</p>
          <div class="signal-line"><i :class="{ moving: socketState === 'open' }"></i></div>
        </div>

        <div class="privacy-note">
          <span>◌</span>
          <p>文件使用临时访问链接传递，聊天内容由服务端安全处理。</p>
        </div>
      </aside>
    </section>
  </main>
</template>
