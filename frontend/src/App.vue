<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import mascotImage from './assets/mascot.png'

// 与后端 IM_AI_BOT_USERNAME 约定一致，用于识别 AI 机器人账号。
const AI_BOT_USERNAME = 'ai_assistant'

const theme = ref(localStorage.getItem('theme') || 'light')
const token = ref(localStorage.getItem('token') || '')
const me = ref(readStoredUser())
const routePath = ref(window.location.pathname === '/chat' ? '/chat' : '/login')
const authMode = ref('login')
const authForm = ref({
  username: localStorage.getItem('last_username') || '',
  nickname: '',
  password: ''
})
const users = ref([])
const groups = ref([])
const selectedPeer = ref(null)
const selectedGroup = ref(null)
const groupMembers = ref([])
const groupModalOpen = ref(false)
const groupName = ref('')
const selectedMemberIDs = ref([])
const creatingGroup = ref(false)
const presenceByUserID = ref({})
const messages = ref([])
const messageSelectionMode = ref(false)
const selectedMessageIDs = ref([])
const deletingMessages = ref(false)
const loadingAllMessages = ref(false)
const searchQuery = ref('')
const searchResults = ref([])
const addFriendOpen = ref(false)
const addFriendQuery = ref('')
const addFriendResults = ref([])
const addFriendLoading = ref(false)
const joinGroupOpen = ref(false)
const joinGroupQuery = ref('')
const joinGroupResults = ref([])
const joinGroupLoading = ref(false)
const friendRequests = ref([])
const groupJoinRequests = ref([])
const composer = ref('')
const replyTarget = ref(null)
const editingMessageID = ref(0)
const typingHint = ref('')
const typingTimer = ref(null)
const aiTyping = ref(false)
const recallClock = ref(Date.now())
const uploadProgress = ref(0)
const imagePreviewURL = ref('')
const selectedFile = ref(null)
const fileInput = ref(null)
const socket = ref(null)
const socketState = ref('idle')
const loading = ref(false)
const syncing = ref(false)
const toast = ref('')
const toastTimer = ref(null)
const messageBox = ref(null)
const editingNickname = ref(false)
const nicknameDraft = ref('')
const savingNickname = ref(false)

const settingsOpen = ref(false)
const settingsNickname = ref('')
const settingsOldPassword = ref('')
const settingsNewPassword = ref('')
const settingsAvatarFile = ref(null)
const settingsAvatarPreview = ref('')
const settingsAvatarInput = ref(null)
const savingSettings = ref(false)

const balance = ref(0)
const redeemOpen = ref(false)
const redeemCode = ref('')
const redeeming = ref(false)
const redPacketOpen = ref(false)
const redPacketAmount = ref('')
const redPacketCount = ref(1)
const redPacketLucky = ref(true)
const redPacketGreeting = ref('恭喜发财，大吉大利')
const sendingRedPacket = ref(false)
const grabbedAmount = ref(0)
const grabbedShow = ref(false)
const redPacketDetails = ref({})
const redPacketDetail = ref(null)
const redPacketDetailOpen = ref(false)
const favoritesOpen = ref(false)
const favoriteMessages = ref([])
const loadingFavorites = ref(false)

const groupManageOpen = ref(false)
const groupManageAvatarFile = ref(null)
const groupManageAvatarPreview = ref('')
const groupManageInviteQuery = ref('')
const groupManageInviteResults = ref([])
const groupManageLoading = ref(false)
const groupAnnouncementDraft = ref('')

let reconnectTimer = null
let presenceRefreshTimer = null
let recallCountdownTimer = null
let aiTypingTimer = null

const isAuthenticated = computed(() => Boolean(token.value && me.value))
const showLoginPage = computed(() => !isAuthenticated.value && routePath.value === '/login')
const showChatPage = computed(() => isAuthenticated.value && routePath.value === '/chat')
const isGroupConversation = computed(() => Boolean(selectedGroup.value))
const displayTitle = computed(() => {
  if (selectedGroup.value) return selectedGroup.value.name
  return selectedPeer.value ? selectedPeer.value.nickname || selectedPeer.value.username : '选择一个会话'
})
const displaySubtitle = computed(() => {
  if (selectedGroup.value) return `${groupMembers.value.length} 位参与者 · 实时同步中`
  return selectedPeer.value ? '安全直连 · 实时同步中' : '选择联系人或群聊开始对话'
})
const currentTitle = computed(() => selectedPeer.value ? selectedPeer.value.nickname || selectedPeer.value.username : '选择一位联系人开始对话')
const currentSubtitle = computed(() => selectedPeer.value ? '安全直连 · 实时同步中' : '从左侧联系人列表开启一次对话')
const socketLabel = computed(() => {
  if (socketState.value === 'open') return '实时连接'
  if (socketState.value === 'connecting') return '正在连线'
  return '离线重连中'
})

function applyThemeToDocument() {
  // Teleport 到 body 的弹窗取不到 .app-shell 作用域内的 CSS 变量，
  // 因此把主题同步到 <html>，让 :root 的变量随之切换。
  document.documentElement.setAttribute('data-theme', theme.value)
}

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  localStorage.setItem('theme', theme.value)
  applyThemeToDocument()
}

function readStoredUser() {
  try {
    return JSON.parse(localStorage.getItem('me') || 'null')
  } catch {
    return null
  }
}

function navigate(path, replace = false) {
  const nextPath = path === '/chat' ? '/chat' : '/login'
  if (window.location.pathname !== nextPath) {
    const method = replace ? 'replaceState' : 'pushState'
    window.history[method]({}, '', nextPath)
  }
  routePath.value = nextPath
}

function syncRoute() {
  navigate(isAuthenticated.value ? '/chat' : '/login', true)
}

function handlePopState() {
  const requestedPath = window.location.pathname === '/chat' ? '/chat' : '/login'
  routePath.value = requestedPath
  if (isAuthenticated.value !== (requestedPath === '/chat')) {
    syncRoute()
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

// ---- 红包 ----
const cents = (n) => (Number(n || 0) / 100).toFixed(2)

async function loadBalance() {
  try {
    const data = await api('/api/balance')
    balance.value = data.balance || 0
  } catch { /* 余额获取失败不打断主流程 */ }
}

async function redeem() {
  const code = redeemCode.value.trim()
  if (!code) { showToast('请输入兑换码'); return }
  redeeming.value = true
  try {
    const data = await api('/api/redeem', { method: 'POST', body: JSON.stringify({ code }) })
    balance.value += data.amount || 0
    showToast('兑换成功 +' + cents(data.amount) + ' 元')
    redeemCode.value = ''
    redeemOpen.value = false
  } catch (error) {
    showToast(error.message)
  } finally {
    redeeming.value = false
  }
}

async function sendRedPacket() {
  const amount = Math.round(Number(redPacketAmount.value) * 100)
  const count = Number(redPacketCount.value)
  if (!amount || amount <= 0) { showToast('请输入有效的金额'); return }
  if (!count || count <= 0) { showToast('请输入有效的个数'); return }
  if (count > amount) { showToast('单个红包金额至少 0.01 元'); return }
  if (!selectedGroup.value) { showToast('请先选择群聊'); return }
  sendingRedPacket.value = true
  try {
    await api('/api/groups/' + selectedGroup.value.id + '/red-packets', {
      method: 'POST',
      body: JSON.stringify({
        amount,
        count,
        lucky: redPacketLucky.value,
        greeting: redPacketGreeting.value || '恭喜发财，大吉大利'
      })
    })
    redPacketOpen.value = false
    redPacketAmount.value = ''
    showToast('红包已发出')
  } catch (error) {
    showToast(error.message)
  } finally {
    sendingRedPacket.value = false
  }
}

async function loadRedPacketDetail(packetID) {
  if (redPacketDetails.value[packetID]) return redPacketDetails.value[packetID]
  const data = await api('/api/red-packets/' + packetID)
  redPacketDetails.value[packetID] = data
  return data
}

function hasGrabbed(message) {
  const info = redPacketInfo(message)
  const detail = redPacketDetails.value[info.id]
  if (!detail || !detail.receipts) return false
  return detail.receipts.some((r) => Number(r.user_id) === Number(me.value?.id))
}

async function grabRedPacket(message) {
  const packetID = redPacketInfo(message).id
  if (!packetID) return
  try {
    const data = await api('/api/red-packets/' + packetID + '/grab', { method: 'POST' })
    grabbedAmount.value = data.receipt.amount
    grabbedShow.value = true
    await refreshRedPacketDetail(packetID)
  } catch (error) {
    showToast(error.message)
  }
}

async function refreshRedPacketDetail(packetID) {
  try {
    const data = await api('/api/red-packets/' + packetID)
    redPacketDetails.value[packetID] = data
  } catch { /* 详情刷新失败不影响主流程 */ }
}

// 批量加载消息列表中的红包详情，用于刷新后正确显示"已领取"状态。
async function preloadRedPacketDetails(list) {
  const ids = [...new Set(
    (list || []).filter((m) => m.content_type === 'red_packet').map((m) => redPacketInfo(m).id).filter(Boolean)
  )]
  await Promise.allSettled(ids.map((id) => refreshRedPacketDetail(id)))
}

// 点击红包：已领过则打开领取详情，未领过则抢。
async function handleRedPacketClick(message) {
  const packetID = redPacketInfo(message).id
  if (!packetID) return
  try {
    const detail = await loadRedPacketDetail(packetID)
    redPacketDetails.value[packetID] = detail
    if (hasGrabbed(message)) {
      redPacketDetail.value = detail
      redPacketDetailOpen.value = true
    } else {
      await grabRedPacket(message)
    }
  } catch (error) {
    showToast(error.message)
  }
}

// 红包消息的 content 是 JSON（{"id":N,"greeting":"..."}），旧数据退化为纯数字 ID。
function redPacketInfo(message) {
  try {
    const obj = JSON.parse(message.content)
    return { id: Number(obj.id), greeting: obj.greeting || '恭喜发财，大吉大利' }
  } catch {
    return { id: Number(message.content), greeting: '恭喜发财，大吉大利' }
  }
}

function redPacketMessage(packetID) {
  return messages.value.find((m) => m.content_type === 'red_packet' && redPacketInfo(m).id === Number(packetID)) || null
}

async function submitAuth() {
  if (!authForm.value.username || !authForm.value.password) {
    showToast('请填写用户名和密码')
    return
  }

  loading.value = true
  try {
    localStorage.setItem('last_username', authForm.value.username)
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
    navigate('/chat')
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
    await Promise.all([refreshUsers(), refreshGroups(), loadOfflineMessages(), refreshRequests(), loadBalance()])
    startPresencePolling()
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
  const nextUsers = data.users || []
  const previousPresence = presenceByUserID.value
  presenceByUserID.value = Object.fromEntries(
    nextUsers.map((user) => [user.id, previousPresence[user.id] ?? null])
  )
  users.value = nextUsers
  await refreshPresence(nextUsers)
}

async function refreshGroups() {
  const data = await api('/api/groups')
  groups.value = data.groups || []
}

async function refreshRequests() {
  const [friends, joins] = await Promise.all([
    api('/api/friend-requests'),
    api('/api/group-join-requests')
  ])
  friendRequests.value = friends.requests || []
  groupJoinRequests.value = joins.requests || []
}

function openAddFriendModal() {
  settingsOpen.value = false
  groupManageOpen.value = false
  joinGroupOpen.value = false
  addFriendOpen.value = true
  addFriendQuery.value = ''
  addFriendResults.value = []
}

function closeAddFriendModal() { addFriendOpen.value = false }

async function searchAddFriend() {
  const query = addFriendQuery.value.trim()
  if (!query) return
  addFriendLoading.value = true
  try {
    const data = await api('/api/search/directory?type=user&q=' + encodeURIComponent(query))
    addFriendResults.value = data.users || []
    if (!addFriendResults.value.length) showToast('未找到匹配用户')
  } catch (error) {
    addFriendResults.value = []
    showToast(error.message)
  } finally { addFriendLoading.value = false }
}

async function sendFriendRequest(user) {
  if (!user) return
  try {
    await api('/api/friend-requests', { method: 'POST', body: JSON.stringify({ to_user_id: user.id }) })
    showToast('好友申请已发送')
    addFriendResults.value = addFriendResults.value.filter((u) => u.id !== user.id)
  } catch (error) { showToast(error.message) }
}

function openJoinGroupModal() {
  settingsOpen.value = false
  groupManageOpen.value = false
  addFriendOpen.value = false
  joinGroupOpen.value = true
  joinGroupQuery.value = ''
  joinGroupResults.value = []
}

function closeJoinGroupModal() { joinGroupOpen.value = false }

async function searchJoinGroup() {
  const query = joinGroupQuery.value.trim()
  if (!query) return
  joinGroupLoading.value = true
  try {
    const data = await api('/api/search/directory?type=group&q=' + encodeURIComponent(query))
    joinGroupResults.value = data.groups || []
    if (!joinGroupResults.value.length) showToast('未找到匹配群聊')
  } catch (error) {
    joinGroupResults.value = []
    showToast(error.message)
  } finally { joinGroupLoading.value = false }
}

async function requestGroupJoin(group) {
  if (!group) return
  try {
    await api('/api/group-join-requests', { method: 'POST', body: JSON.stringify({ group_id: group.id }) })
    showToast('入群申请已发送')
    joinGroupResults.value = joinGroupResults.value.filter((g) => g.id !== group.id)
  } catch (error) { showToast(error.message) }
}

async function respondFriendRequest(request, accept) {
  try {
    await api('/api/friend-requests/' + request.id + '/respond', { method: 'POST', body: JSON.stringify({ accept }) })
    await Promise.all([refreshRequests(), refreshUsers()])
    showToast(accept ? '已添加好友' : '已拒绝申请')
  } catch (error) { showToast(error.message) }
}

async function respondGroupJoinRequest(request, accept) {
  try {
    await api('/api/group-join-requests/' + request.id + '/respond', { method: 'POST', body: JSON.stringify({ accept }) })
    await Promise.all([refreshRequests(), refreshGroups()])
    showToast(accept ? '已同意入群' : '已拒绝申请')
  } catch (error) { showToast(error.message) }
}

async function refreshPresence(targetUsers = users.value) {
  const results = await Promise.allSettled(
    targetUsers.map(async (user) => {
      const data = await api('/api/presence?user_id=' + encodeURIComponent(user.id))
      return [user.id, data.online === true]
    })
  )

  const nextPresence = { ...presenceByUserID.value }
  let unauthorizedError = null
  for (const result of results) {
    if (result.status === 'fulfilled') {
      const [userID, online] = result.value
      nextPresence[userID] = online
    } else if (result.reason?.status === 401) {
      unauthorizedError = result.reason
    }
  }
  presenceByUserID.value = nextPresence
  if (unauthorizedError) throw unauthorizedError
}

function startPresencePolling() {
  window.clearInterval(presenceRefreshTimer)
  presenceRefreshTimer = window.setInterval(() => {
    if (!isAuthenticated.value || users.value.length === 0) return
    refreshPresence().catch((error) => {
      if (error.status === 401) {
        clearSession()
        showToast('登录状态已失效，请重新登录')
      }
    })
  }, 30000)
}

function isUserOnline(user) {
  if (isAIBot(user)) return true
  return Boolean(user && presenceByUserID.value[user.id] === true)
}

function isUserOffline(user) {
  if (isAIBot(user)) return false
  return Boolean(user && presenceByUserID.value[user.id] === false)
}

function presenceLabel(user) {
  if (isAIBot(user)) return '在线'
  if (!user || presenceByUserID.value[user.id] === null || presenceByUserID.value[user.id] === undefined) {
    return '状态同步中'
  }
  return isUserOnline(user) ? '在线' : '离线'
}

async function selectPeer(user) {
  selectedPeer.value = user
  selectedGroup.value = null
  groupMembers.value = []
  searchResults.value = []
  // Clear the visible badge immediately; the conversation request below
  // performs the durable Redis read-mark on the server.
  clearLocalUnread(user.id)
  await loadConversation()
  await refreshUsers()
  clearLocalUnread(user.id)
}

function clearLocalUnread(userID) {
  users.value = users.value.map((user) => (
    Number(user.id) === Number(userID) ? { ...user, unread_count: 0 } : user
  ))
}

function markConversationRead() {
  const last = messages.value[messages.value.length - 1]
  if (!last) return
  const payload = { message_id: last.id }
  if (selectedGroup.value) payload.group_id = selectedGroup.value.id
  else if (selectedPeer.value) payload.peer_id = selectedPeer.value.id
  api('/api/messages/read', { method: 'POST', body: JSON.stringify(payload) }).catch(() => {})
}

async function selectGroup(group) {
  selectedGroup.value = group
  selectedPeer.value = null
  searchResults.value = []
  try {
    const [messagesData, membersData] = await Promise.all([
      api('/api/groups/' + group.id + '/messages?limit=100'),
      api('/api/groups/' + group.id + '/members')
    ])
    messages.value = messagesData.messages || []
    messageSelectionMode.value = false
    selectedMessageIDs.value = []
    groupMembers.value = membersData.members || []
    preloadRedPacketDetails(messages.value)
    await scrollMessages()
    markConversationRead()
  } catch (error) {
    showToast(error.message)
  }
}

function openGroupModal() {
  groupName.value = ''
  selectedMemberIDs.value = []
  groupModalOpen.value = true
}

function closeGroupModal() {
  if (!creatingGroup.value) groupModalOpen.value = false
}

async function createGroup() {
  const name = groupName.value.trim()
  if (!name) {
    showToast('请输入群名称')
    return
  }
  creatingGroup.value = true
  try {
    const data = await api('/api/groups', {
      method: 'POST',
      body: JSON.stringify({ name, member_ids: selectedMemberIDs.value })
    })
    await refreshGroups()
    const created = groups.value.find((item) => item.id === data.group?.id) || data.group
    groupModalOpen.value = false
    if (created) await selectGroup(created)
    showToast('群聊创建成功')
  } catch (error) {
    showToast(error.message)
  } finally {
    creatingGroup.value = false
  }
}

async function loadConversation() {
  if (selectedGroup.value) {
    await selectGroup(selectedGroup.value)
    return
  }
  if (!selectedPeer.value) return
  try {
    const data = await api('/api/messages?peer_id=' + selectedPeer.value.id + '&limit=100')
    messages.value = data.messages || []
    selectedMessageIDs.value = []
    await scrollMessages()
    markConversationRead()
  } catch (error) {
    showToast(error.message)
  }
}

const selectableMessages = computed(() => messages.value)
const allMessagesSelected = computed(() => selectableMessages.value.length > 0 && selectableMessages.value.every((message) => selectedMessageIDs.value.includes(message.id)))

function toggleMessageSelectionMode() {
  messageSelectionMode.value = !messageSelectionMode.value
  selectedMessageIDs.value = []
}

function toggleAllOwnMessages() {
  selectedMessageIDs.value = allMessagesSelected.value ? [] : selectableMessages.value.map((message) => message.id)
}

// 会话初始只加载最近一页消息；"全选所有消息"需要先把整个会话翻页拉全。
function conversationMessagesURL(afterID) {
  if (selectedGroup.value) return '/api/groups/' + selectedGroup.value.id + '/messages?after_id=' + afterID + '&limit=100'
  if (selectedPeer.value) return '/api/messages?peer_id=' + selectedPeer.value.id + '&after_id=' + afterID + '&limit=100'
  return ''
}

async function loadEntireConversation() {
  if (!conversationMessagesURL(0) || loadingAllMessages.value) return
  loadingAllMessages.value = true
  try {
    const all = []
    let afterID = 0
    for (;;) {
      const data = await api(conversationMessagesURL(afterID))
      const batch = data.messages || []
      all.push(...batch)
      if (batch.length < 100) break
      afterID = batch[batch.length - 1].id
    }
    messages.value = all
    await scrollMessages()
  } catch (error) {
    showToast(error.message)
  } finally {
    loadingAllMessages.value = false
  }
}

async function toggleAllMessages() {
  if (allMessagesSelected.value) {
    selectedMessageIDs.value = []
    return
  }
  await loadEntireConversation()
  selectedMessageIDs.value = selectableMessages.value.map((message) => message.id)
}

async function deleteSelectedMessages() {
  if (!selectedMessageIDs.value.length) return
  if (!confirm('确定删除选中的 ' + selectedMessageIDs.value.length + ' 条消息吗？此操作无法恢复。')) return
  await deleteMessages({ message_ids: selectedMessageIDs.value })
}

async function deleteAllOwnMessages() {
  if (!selectableMessages.value.length) return
  if (!confirm('确定删除当前会话中的全部 ' + selectableMessages.value.length + ' 条消息吗？此操作无法恢复。')) return
  await deleteMessages({ all: true })
}

async function deleteMessages(payload) {
  if (!selectedPeer.value && !selectedGroup.value) return
  deletingMessages.value = true
  try {
    if (selectedGroup.value) payload.group_id = selectedGroup.value.id
    else payload.peer_id = selectedPeer.value.id
    const data = await api('/api/messages', { method: 'DELETE', body: JSON.stringify(payload) })
    const deletedIDs = new Set(payload.message_ids || selectableMessages.value.map((message) => message.id))
    messages.value = messages.value.filter((message) => !deletedIDs.has(message.id))
    selectedMessageIDs.value = []
    messageSelectionMode.value = false
    showToast('已删除 ' + (data.deleted || 0) + ' 条消息')
  } catch (error) {
    showToast(error.message)
  } finally {
    deletingMessages.value = false
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
    if (payload.type === 'recall') {
      applyRecall(payload.message)
      return
    }
    if (payload.type === 'edit') {
      applyEdit(payload.message)
      return
    }
    if (payload.type === 'typing') {
      const current = payload.group_id
        ? Number(payload.group_id) === Number(selectedGroup.value?.id)
        : Number(payload.to) === Number(selectedPeer.value?.id)
      if (current) {
        typingHint.value = payload.group_id ? '有成员正在输入...' : '对方正在输入...'
        window.setTimeout(() => { typingHint.value = '' }, 1400)
      }
      return
    }
    if (payload.type === 'chat' || payload.type === 'ack') {
      if (payload.type === 'chat' && selectedPeer.value && isAIBot(selectedPeer.value) && Number(payload.message?.from_id) === Number(selectedPeer.value.id)) {
        aiTyping.value = false
        window.clearTimeout(aiTypingTimer)
      }
      appendMessage(payload.message)
      if (payload.type === 'chat') {
        if (isCurrentPrivateMessage(payload.message)) {
          await loadConversation()
          clearLocalUnread(selectedPeer.value.id)
        }
        await refreshUsers()
        if (isCurrentPrivateMessage(payload.message)) clearLocalUnread(selectedPeer.value.id)
      }
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

function isCurrentPrivateMessage(message) {
  if (!message || !me.value || !selectedPeer.value || selectedGroup.value) return false
  return (
    (message.from_id === me.value.id && message.to_id === selectedPeer.value.id) ||
    (message.from_id === selectedPeer.value.id && message.to_id === me.value.id)
  )
}

function appendMessage(message) {
  if (!message) return
  if (selectedGroup.value) {
    if (Number(message.group_id) !== Number(selectedGroup.value.id)) return
    if (messages.value.some((item) => item.id === message.id)) return
    messages.value.push(message)
    messages.value.sort((left, right) => left.id - right.id)
    scrollMessages()
    markConversationRead()
    return
  }
  if (!selectedPeer.value) return
  const inCurrentConversation =
    (message.from_id === me.value.id && message.to_id === selectedPeer.value.id) ||
    (message.from_id === selectedPeer.value.id && message.to_id === me.value.id)

  if (!inCurrentConversation || messages.value.some((item) => item.id === message.id)) return
  messages.value.push(message)
  messages.value.sort((left, right) => left.id - right.id)
  scrollMessages()
  markConversationRead()
}

function applyRecall(message) {
  if (!message) return
  const target = messages.value.find((item) => Number(item.id) === Number(message.id))
  if (!target) return
  target.recalled_at = message.recalled_at
  target.recalled_by = message.recalled_by
  target.content = ''
  target.object_url = ''
  target.object_key = ''
  target.file_name = ''
  target.file_size = 0
}

function applyEdit(message) {
  const target = messages.value.find((item) => Number(item.id) === Number(message?.id))
  if (!target) return
  target.content = message.content
  target.edited_at = message.edited_at
}

function startReply(message) {
  if (!message || message.recalled_at) return
  replyTarget.value = message
  editingMessageID.value = 0
}

function cancelReply() { replyTarget.value = null }

function startEdit(message) {
  if (!isMine(message) || message.recalled_at || message.content_type !== 'text') return
  editingMessageID.value = message.id
  replyTarget.value = null
  composer.value = message.content || ''
}

async function saveEdit() {
  const content = composer.value.trim()
  if (!editingMessageID.value || !content) return
  try {
    const data = await api('/api/messages/' + editingMessageID.value, { method: 'PATCH', body: JSON.stringify({ content }) })
    applyEdit(data.message)
    composer.value = ''
    editingMessageID.value = 0
    showToast('消息已编辑')
  } catch (error) { showToast(error.message) }
}

async function toggleFavorite(message) {
  try {
    const enabled = !message.is_favorite
    await api('/api/messages/' + message.id + '/favorite', { method: 'PUT', body: JSON.stringify({ enabled }) })
    message.is_favorite = enabled
  } catch (error) { showToast(error.message) }
}

// 打开收藏列表，拉取当前会话的收藏消息。
async function openFavorites() {
  favoritesOpen.value = true
  loadingFavorites.value = true
  favoriteMessages.value = []
  try {
    const params = selectedGroup.value
      ? '?group_id=' + selectedGroup.value.id
      : (selectedPeer.value ? '?peer_id=' + selectedPeer.value.id : '')
    const data = await api('/api/messages/favorites' + params)
    favoriteMessages.value = data.messages || []
  } catch (error) {
    showToast(error.message)
  } finally {
    loadingFavorites.value = false
  }
}

// 关闭收藏弹窗并跳转到目标消息。
function jumpToMessage(message) {
  favoritesOpen.value = false
  scrollToMessage(message.id)
}

// 滚动到指定消息并高亮。
function scrollToMessage(messageID) {
  const el = document.getElementById('msg-' + messageID)
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
    el.classList.add('flash-highlight')
    window.setTimeout(() => el.classList.remove('flash-highlight'), 2200)
  } else {
    showToast('原消息不在当前会话中或已删除')
  }
}

function previewImage(message) { imagePreviewURL.value = message?.object_url || '' }

function recallSecondsLeft(message) {
  if (!isMine(message) || message.recalled_at) return 0
  const created = new Date(message.created_at).getTime()
  return Math.max(0, 120 - Math.floor((recallClock.value - created) / 1000))
}

function sendTyping() {
  if (!socket.value || socket.value.readyState !== WebSocket.OPEN || (!selectedPeer.value && !selectedGroup.value)) return
  if (typingTimer.value) return
  const payload = { type: 'typing' }
  if (selectedGroup.value) payload.group_id = selectedGroup.value.id
  else payload.to = selectedPeer.value.id
  socket.value.send(JSON.stringify(payload))
  typingTimer.value = window.setTimeout(() => { typingTimer.value = null }, 1200)
}

async function recallMessage(message) {
  if (!message || !isMine(message) || message.recalled_at) return
  if (!confirm('确定撤回这条消息吗？')) return
  try {
    const data = await api('/api/messages/' + message.id + '/recall', { method: 'POST' })
    applyRecall(data.message)
  } catch (error) { showToast(error.message) }
}

function sendMessage() {
  const content = composer.value.trim()
  if (!selectedPeer.value && !selectedGroup.value) {
    showToast('请先选择一位联系人')
    return
  }
  if (!content) return
  if (editingMessageID.value) {
    saveEdit()
    return
  }
  if (!socket.value || socket.value.readyState !== WebSocket.OPEN) {
    showToast('实时连接尚未就绪')
    return
  }

  const payload = { type: 'chat', content }
  if (replyTarget.value) payload.reply_to_id = replyTarget.value.id
  if (selectedGroup.value) payload.group_id = selectedGroup.value.id
  else payload.to = selectedPeer.value.id
  socket.value.send(JSON.stringify(payload))
  if (selectedPeer.value && isAIBot(selectedPeer.value)) {
    aiTyping.value = true
    window.clearTimeout(aiTypingTimer)
    aiTypingTimer = window.setTimeout(() => { aiTyping.value = false }, 60000)
  }
  composer.value = ''
  replyTarget.value = null
}

async function uploadMedia() {
  if (selectedGroup.value) {
    showToast('群文件上传暂未开放')
    return
  }
  if (!selectedPeer.value) {
    showToast('请先选择一位联系人')
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
  uploadProgress.value = 8
  try {
    const data = await uploadMediaRequest(form)
    appendMessage(data.message)
    showToast('文件已发送')
    selectedFile.value = null
    if (fileInput.value) fileInput.value.value = ''
  } catch (error) {
    showToast(error.message)
  } finally {
    loading.value = false
    uploadProgress.value = 0
  }
}

function uploadMediaRequest(form) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/media/upload')
    xhr.setRequestHeader('Authorization', 'Bearer ' + token.value)
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) uploadProgress.value = Math.max(8, Math.round(event.loaded / event.total * 100))
    }
    xhr.onerror = () => reject(new Error('文件上传失败'))
    xhr.onload = () => {
      let data = {}
      try { data = JSON.parse(xhr.responseText || '{}') } catch {}
      if (xhr.status < 200 || xhr.status >= 300) {
        const error = new Error(data.error || '文件上传失败')
        error.status = xhr.status
        reject(error)
        return
      }
      uploadProgress.value = 100
      resolve(data)
    }
    xhr.send(form)
  })
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
    const scopeSuffix = selectedGroup.value
      ? '&group_id=' + selectedGroup.value.id
      : (selectedPeer.value ? '&peer_id=' + selectedPeer.value.id : '')
    const data = await api('/api/search/messages?q=' + encodeURIComponent(query) + scopeSuffix + '&limit=50')
    searchResults.value = data.messages || []
  } catch (error) {
    showToast(error.message)
  }
}

async function openSearchResult(message) {
  if (message.group_id) {
    const group = groups.value.find((item) => Number(item.id) === Number(message.group_id))
    if (group) await selectGroup(group)
    return
  }
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

function beginNicknameEdit() {
  nicknameDraft.value = me.value?.nickname || me.value?.username || ''
  editingNickname.value = true
}

function cancelNicknameEdit() {
  editingNickname.value = false
  nicknameDraft.value = ''
}

function openSettings() {
  settingsNickname.value = me.value?.nickname || ''
  settingsOldPassword.value = ''
  settingsNewPassword.value = ''
  settingsAvatarFile.value = null
  settingsAvatarPreview.value = ''
  savingSettings.value = false
  settingsOpen.value = true
}

function closeSettings() {
  settingsOpen.value = false
}

function pickAvatarFile(event) {
  const file = event.target.files && event.target.files[0]
  if (!file) return
  if (!file.type.startsWith('image/')) {
    showToast('仅支持图片文件')
    return
  }
  settingsAvatarFile.value = file
  const reader = new FileReader()
  reader.onload = (e) => { settingsAvatarPreview.value = e.target.result }
  reader.readAsDataURL(file)
}

async function saveSettings() {
  savingSettings.value = true
  try {
    if (settingsAvatarFile.value) {
      const form = new FormData()
      form.append('avatar', settingsAvatarFile.value)
      const data = await api('/api/me/avatar', { method: 'POST', body: form })
      me.value = data.user
      localStorage.setItem('me', JSON.stringify(data.user))
    }
    const nickname = settingsNickname.value.trim()
    if (nickname && nickname !== me.value.nickname) {
      const data = await api('/api/me', { method: 'PATCH', body: JSON.stringify({ nickname }) })
      me.value = data.user
      localStorage.setItem('me', JSON.stringify(data.user))
      await refreshUsers()
    }
    if (settingsOldPassword.value && settingsNewPassword.value) {
      await api('/api/me/password', { method: 'POST', body: JSON.stringify({ old_password: settingsOldPassword.value, new_password: settingsNewPassword.value }) })
      showToast('密码已修改')
    }
    settingsOpen.value = false
    if (nickname && nickname !== me.value.nickname) {
      showToast('设置已保存')
    }
  } catch (error) {
    if (error.status === 401) {
      clearSession()
      showToast('登录状态已失效，请重新登录')
      return
    }
    showToast(error.message)
  } finally {
    savingSettings.value = false
  }
}

function saveNickname() {}

async function deleteFriend(user) {
  if (!user || !confirm('确定要删除好友 ' + (user.nickname || user.username) + ' 吗？')) return
  try {
    await api('/api/friends/' + user.id, { method: 'DELETE' })
    await Promise.all([refreshUsers(), refreshGroups()])
    showToast('已删除好友')
  } catch (error) {
    showToast(error.message)
  }
}

function openGroupManage() {
  groupManageOpen.value = true
  groupManageAvatarFile.value = null
  groupManageAvatarPreview.value = ''
  groupManageInviteQuery.value = ''
  groupManageInviteResults.value = []
  groupAnnouncementDraft.value = selectedGroup.value?.announcement || ''
}

function closeGroupManage() {
  groupManageOpen.value = false
}

function pickGroupAvatar(event) {
  const file = event.target.files && event.target.files[0]
  if (!file) return
  if (!file.type.startsWith('image/')) { showToast('仅支持图片文件'); return }
  groupManageAvatarFile.value = file
  const reader = new FileReader()
  reader.onload = (e) => { groupManageAvatarPreview.value = e.target.result }
  reader.readAsDataURL(file)
}

async function saveGroupAvatar() {
  if (!groupManageAvatarFile.value) return
  groupManageLoading.value = true
  try {
    const form = new FormData()
    form.append('avatar', groupManageAvatarFile.value)
    const data = await api('/api/groups/' + selectedGroup.value.id + '/avatar', { method: 'POST', body: form })
    selectedGroup.value = data.group
    await refreshGroups()
    showToast('群头像已更新')
  } catch (error) {
    showToast(error.message)
  } finally {
    groupManageLoading.value = false
  }
}

async function searchUserToInvite() {
  const query = groupManageInviteQuery.value.trim()
  if (!query) { groupManageInviteResults.value = []; return }
  groupManageLoading.value = true
  try {
    const data = await api('/api/search/directory?type=user&q=' + encodeURIComponent(query))
    groupManageInviteResults.value = data.users || []
  } catch (error) {
    showToast(error.message)
  } finally {
    groupManageLoading.value = false
  }
}

async function inviteGroupMember(user) {
  groupManageLoading.value = true
  try {
    await api('/api/groups/' + selectedGroup.value.id + '/members', { method: 'POST', body: JSON.stringify({ user_id: user.id }) })
    const memberData = await api('/api/groups/' + selectedGroup.value.id + '/members')
    groupMembers.value = memberData.members || []
    groupManageInviteQuery.value = ''
    groupManageInviteResults.value = []
    showToast('已邀请参与者加入')
  } catch (error) {
    showToast(error.message)
  } finally {
    groupManageLoading.value = false
  }
}

async function kickGroupMember(userID) {
  if (!confirm('确定要将该参与者移出群聊吗？')) return
  groupManageLoading.value = true
  try {
    await api('/api/groups/' + selectedGroup.value.id + '/members/' + userID, { method: 'DELETE' })
    const memberData = await api('/api/groups/' + selectedGroup.value.id + '/members')
    groupMembers.value = memberData.members || []
    showToast('已移出群聊')
  } catch (error) {
    showToast(error.message)
  } finally {
    groupManageLoading.value = false
  }
}

async function dissolveGroup() {
  const group = selectedGroup.value
  if (!group) return
  if (!confirm('确定要解散“' + group.name + '”吗？此操作会永久删除群消息和参与者关系，无法恢复。')) return

  groupManageLoading.value = true
  try {
    await api('/api/groups/' + group.id, { method: 'DELETE' })
    groupManageOpen.value = false
    selectedGroup.value = null
    groupMembers.value = []
    messages.value = []
    await refreshGroups()
    showToast('群聊已解散')
  } catch (error) {
    showToast(error.message)
  } finally {
    groupManageLoading.value = false
  }
}

function isGroupManager() {
  const mine = groupMembers.value.find((member) => Number(member.user_id) === Number(me.value?.id))
  return mine?.role === 'owner' || mine?.role === 'admin'
}

async function saveGroupAnnouncement() {
  if (!selectedGroup.value) return
  groupManageLoading.value = true
  try {
    const data = await api('/api/groups/' + selectedGroup.value.id + '/announcement', { method: 'PATCH', body: JSON.stringify({ announcement: groupAnnouncementDraft.value }) })
    selectedGroup.value = data.group
    await refreshGroups()
    showToast('群公告已保存')
  } catch (error) { showToast(error.message) } finally { groupManageLoading.value = false }
}

async function toggleAllMuted() {
  if (!selectedGroup.value) return
  groupManageLoading.value = true
  try {
    const data = await api('/api/groups/' + selectedGroup.value.id + '/all-muted', { method: 'PUT', body: JSON.stringify({ muted: !selectedGroup.value.all_muted }) })
    selectedGroup.value = data.group
    await refreshGroups()
  } catch (error) { showToast(error.message) } finally { groupManageLoading.value = false }
}

async function setGroupMemberRole(member, role) {
  try {
    await api('/api/groups/' + selectedGroup.value.id + '/members/' + member.user_id + '/role', { method: 'PUT', body: JSON.stringify({ role }) })
    const data = await api('/api/groups/' + selectedGroup.value.id + '/members')
    groupMembers.value = data.members || []
  } catch (error) { showToast(error.message) }
}

async function muteGroupMember(member) {
  const raw = prompt('禁言分钟数，输入 0 解除禁言', member.muted_until ? '0' : '10')
  if (raw === null) return
  const minutes = Number(raw)
  if (!Number.isInteger(minutes) || minutes < 0) { showToast('请输入有效分钟数'); return }
  try {
    await api('/api/groups/' + selectedGroup.value.id + '/members/' + member.user_id + '/mute', { method: 'PUT', body: JSON.stringify({ minutes }) })
    const data = await api('/api/groups/' + selectedGroup.value.id + '/members')
    groupMembers.value = data.members || []
  } catch (error) { showToast(error.message) }
}

async function leaveGroup() {
  if (!selectedGroup.value || !confirm('确定退出当前群聊吗？')) return
  try {
    await api('/api/groups/' + selectedGroup.value.id + '/leave', { method: 'POST' })
    groupManageOpen.value = false
    selectedGroup.value = null
    messages.value = []
    groupMembers.value = []
    await refreshGroups()
    showToast('已退出群聊')
  } catch (error) { showToast(error.message) }
}

function clearSession() {
  const retainedUsername = authForm.value.username || me.value?.username || ''
  localStorage.setItem('last_username', retainedUsername)
  window.clearTimeout(reconnectTimer)
  window.clearTimeout(aiTypingTimer)
  aiTyping.value = false
  window.clearInterval(presenceRefreshTimer)
  if (socket.value) {
    socket.value.onclose = null
    socket.value.close()
  }
  socket.value = null
  token.value = ''
  me.value = null
  authMode.value = 'login'
  authForm.value = {
    username: retainedUsername,
    nickname: '',
    password: ''
  }
  users.value = []
  groups.value = []
  presenceByUserID.value = {}
  selectedPeer.value = null
  selectedGroup.value = null
  groupMembers.value = []
  messages.value = []
  searchResults.value = []
  localStorage.removeItem('token')
  localStorage.removeItem('me')
  navigate('/login', true)
}

function avatarText(user) {
  const label = user && (user.nickname || user.username) ? user.nickname || user.username : '?'
  return Array.from(label).slice(0, 1).join('').toUpperCase()
}

function isAIBot(user) {
  return Boolean(user && user.username === AI_BOT_USERNAME)
}

function displayName(user) {
  return user ? user.nickname || user.username : ''
}

function userNameByID(userID) {
  const user = users.value.find((item) => Number(item.id) === Number(userID))
  return displayName(user) || ('用户 ' + userID)
}

function requesterName(request) {
  return request.from_nickname || request.from_username || userNameByID(request.from_user_id)
}

function applicantName(request) {
  return request.nickname || request.username || userNameByID(request.user_id)
}

function avatarStyle(user) {
  const label = user && (user.nickname || user.username) ? user.nickname || user.username : '?'
  let hash = 0
  for (const character of label) hash = (hash * 31 + character.codePointAt(0)) % 360
  return { '--avatar-hue': hash }
}

function isMine(message) {
  return Boolean(me.value && Number(message?.from_id) === Number(me.value.id))
}

function messageAuthor(message) {
  if (isMine(message)) return me.value
  return users.value.find((user) => user.id === message.from_id) || {
    id: message.from_id,
    username: 'User ' + message.from_id,
    nickname: 'User ' + message.from_id
  }
}

function messageAuthorName(message) {
  const author = messageAuthor(message)
  return isMine(message) ? 'Me' : author.nickname || author.username
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
  applyThemeToDocument()
  window.addEventListener('popstate', handlePopState)
  recallCountdownTimer = window.setInterval(() => {
    recallClock.value = Date.now()
  }, 1000)
  syncRoute()
  if (isAuthenticated.value) bootstrapWorkspace()
})

onBeforeUnmount(() => {
  window.removeEventListener('popstate', handlePopState)
  window.clearTimeout(reconnectTimer)
  window.clearInterval(presenceRefreshTimer)
  window.clearInterval(recallCountdownTimer)
  if (socket.value) socket.value.close()
})
</script>

<template>
  <main class="app-shell" :data-theme="theme">
    <div class="sky-deco" aria-hidden="true">
      <div class="aurora aurora-one"></div>
      <div class="aurora aurora-two"></div>
      <div class="aurora aurora-three"></div>
      <div class="ray-field"></div>
      <div class="halftone-field"></div>
      <div class="horizon-glow"></div>
      <div class="star-field">
        <span class="tw tw-1">✦</span>
        <span class="tw tw-2">✧</span>
        <span class="tw tw-3">✦</span>
        <span class="tw tw-4">✧</span>
        <span class="tw tw-5">✦</span>
        <span class="tw tw-6">✧</span>
        <span class="tw tw-7">✦</span>
        <span class="tw tw-8">✧</span>
        <span class="tw tw-9">✦</span>
        <span class="tw tw-10">✧</span>
      </div>
      <div class="petal-field">
        <span class="pt pt-1"></span>
        <span class="pt pt-2"></span>
        <span class="pt pt-3"></span>
        <span class="pt pt-4"></span>
        <span class="pt pt-5"></span>
        <span class="pt pt-6"></span>
        <span class="pt pt-7"></span>
        <span class="pt pt-8"></span>
        <span class="pt pt-9"></span>
      </div>
    </div>

    <button
      class="theme-toggle"
      type="button"
      :aria-label="theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'"
      :title="theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'"
      @click="toggleTheme"
    >{{ theme === 'dark' ? '☀' : '☾' }}</button>

    <Transition name="toast">
      <div v-if="toast" class="toast-message">
        <span class="toast-star">✦</span>
        {{ toast }}
      </div>
    </Transition>

    <section v-if="showLoginPage" class="welcome-page">
      <div class="welcome-visual">
        <div class="mascot mascot-large">
          <img class="mascot-img" :src="mascotImage" alt="星讯吉祥物" />
        </div>
        <div class="welcome-copy">
          <p class="eyebrow">PRIVATE SIGNAL / 01</p>
          <h1 class="hero-brand">
            <span>星</span><em>讯</em>
          </h1>
          <p class="welcome-description">
            为每一次灵感、确认与回应，留下有温度的实时轨迹。
          </p>
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
          <input v-model.trim="authForm.username" autocomplete="username" placeholder="输入用户名" />
        </label>

        <label v-if="authMode === 'register'" class="field-label">
          <span>昵称</span>
          <input v-model.trim="authForm.nickname" placeholder="显示给其他人的名字" />
        </label>

        <label class="field-label">
          <span>密码</span>
          <input v-model="authForm.password" type="password" autocomplete="current-password" placeholder="至少 6 个字符" />
        </label>

        <button class="primary-action" type="submit" :disabled="loading">
          <span>{{ loading ? '正在连接星图…' : authMode === 'login' ? '进入工作台' : '创建账号' }}</span>
          <b>→</b>
        </button>

        <p class="auth-note">登录即表示进入你的个人消息空间。</p>
      </form>
    </section>

    <section v-else-if="showChatPage" class="workspace">
      <aside class="left-panel">
        <div class="brand-lockup">
          <div class="brand-star">✦</div>
          <div>
            <strong>星讯</strong>
            <span>PRIVATE CHAT</span>
          </div>
        </div>

        <div class="identity-card">
          <div class="avatar avatar-large" :style="avatarStyle(me)">
            <img v-if="me.avatar_url" :src="me.avatar_url" :alt="me.nickname || me.username" />
            <span v-else>{{ avatarText(me) }}</span>
          </div>
          <div class="identity-copy">
            <strong>{{ me.nickname || me.username }}</strong>
            <span>@{{ me.username }}</span>
            <span class="identity-id">#{{ me.id }}</span>
          </div>
          <button class="icon-button" aria-label="设置" title="个人设置" @click="openSettings">⚙</button>
          <button class="icon-button" aria-label="退出登录" title="退出登录" @click="signOut">↗</button>
        </div>
        <button class="balance-chip" type="button" title="我的钱包" @click="redeemOpen = true">
          <span class="balance-label">💰 钱包</span>
        </button>

        <div class="member-heading">
          <div>
            <p>好友</p>
            <strong>我的好友</strong>
          </div>
          <button class="refresh-button" :class="{ spinning: syncing }" aria-label="刷新好友" @click="refreshUsers">↻</button>
        </div>

        <div class="action-btns">
          <button class="action-btn" @click="openAddFriendModal">添加好友</button>
          <button class="action-btn" @click="openJoinGroupModal">加入群聊</button>
        </div>

        <div v-if="friendRequests.length || groupJoinRequests.length" class="request-list">
          <div v-for="request in friendRequests" :key="'friend-' + request.id" class="request-item">
            <span>{{ requesterName(request) }}(#{{ request.from_user_id }}) 请求加你为好友</span>
            <span class="request-actions">
              <button type="button" @click="respondFriendRequest(request, true)">同意</button>
              <button type="button" @click="respondFriendRequest(request, false)">拒绝</button>
            </span>
          </div>
          <div v-for="request in groupJoinRequests" :key="'join-' + request.id" class="request-item">
            <span>{{ applicantName(request) }}(#{{ request.user_id }}) 申请加入群 {{ request.group_name || ('#' + request.group_id) }}</span>
            <span class="request-actions">
              <button type="button" @click="respondGroupJoinRequest(request, true)">同意</button>
              <button type="button" @click="respondGroupJoinRequest(request, false)">拒绝</button>
            </span>
          </div>
        </div>

        <div class="member-list">
          <button
            v-for="user in users"
            :key="user.id"
            class="member-item"
            :class="{ active: selectedPeer && selectedPeer.id === user.id }"
            @click="selectPeer(user)"
          >
            <div class="member-avatar-wrap">
              <div class="avatar" :style="avatarStyle(user)">
                <img v-if="user.avatar_url" :src="user.avatar_url" :alt="user.nickname || user.username" />
                <span v-else>{{ avatarText(user) }}</span>
              </div>
              <span
                class="presence-dot member-presence"
                :class="{ online: isUserOnline(user), offline: isUserOffline(user) }"
                :title="presenceLabel(user)"
              ></span>
            </div>
            <span class="member-name">{{ user.nickname || user.username }}</span>
            <span v-if="isAIBot(user)" class="ai-badge">AI</span>
            <span class="member-id">#{{ user.id }}</span>
            <button class="member-delete" title="删除好友" @click.stop="deleteFriend(user)">×</button>
            <b v-if="user.unread_count" class="unread-badge">{{ user.unread_count > 99 ? '99+' : user.unread_count }}</b>
          </button>
          <div v-if="!users.length" class="empty-members">还没有联系人。</div>
        </div>

        <div class="group-heading">
          <div>
            <p>群聊</p>
            <strong>我的群聊</strong>
          </div>
          <button class="refresh-button" type="button" aria-label="新建群聊" title="新建群聊" @click="openGroupModal">+</button>
        </div>

        <div class="group-list">
          <button
            v-for="group in groups"
            :key="group.id"
            type="button"
            class="group-item"
            :class="{ active: selectedGroup && selectedGroup.id === group.id }"
            @click="selectGroup(group)"
          >
            <span class="group-mark">
              <img v-if="group.avatar_url" :src="group.avatar_url" :alt="group.name" class="group-avatar-img" />
              <span v-else>#</span>
            </span>
            <span class="group-name">{{ group.name }}</span>
            <span class="group-id">#{{ group.id }}</span>
            <span class="group-count">{{ group.owner_id === me.id ? '群主' : '参与者' }}</span>
          </button>
          <div v-if="!groups.length" class="empty-groups">暂无群聊</div>
        </div>

        <div class="connection-chip">
          <span :class="{ online: socketState === 'open' }"></span>
          {{ socketLabel }}
        </div>
      </aside>

      <section class="chat-panel">
        <header class="chat-header">
          <div class="chat-person">
            <template v-if="selectedPeer && !selectedGroup">
              <div class="avatar avatar-header" :style="avatarStyle(selectedPeer)">
                <img v-if="selectedPeer.avatar_url" :src="selectedPeer.avatar_url" :alt="selectedPeer.nickname || selectedPeer.username" />
                <span v-else>{{ avatarText(selectedPeer) }}</span>
              </div>
              <div>
                <p class="chat-presence">
                  <span class="presence-dot" :class="{ online: isUserOnline(selectedPeer), offline: isUserOffline(selectedPeer) }"></span>
                  {{ presenceLabel(selectedPeer) }} · {{ currentSubtitle }}
                </p>
                <h2>{{ displayTitle }} <span v-if="isAIBot(selectedPeer)" class="ai-badge ai-badge-lg">AI 好友</span></h2>
              </div>
            </template>
            <template v-else-if="selectedGroup">
              <div class="avatar avatar-header group-avatar">
                <img v-if="selectedGroup.avatar_url" :src="selectedGroup.avatar_url" :alt="selectedGroup.name" />
                <span v-else>#</span>
              </div>
              <div>
                <p class="chat-presence">{{ groupMembers.length }} 位参与者 · {{ displaySubtitle }}</p>
                <h2>{{ displayTitle }}</h2>
              </div>
              <button v-if="selectedGroup && selectedGroup.owner_id === me.id" class="group-manage-btn" @click="openGroupManage">管理</button>
            </template>
            <template v-else>
              <div class="compass-mark">✧</div>
              <div>
                <p>{{ displaySubtitle }}</p>
                <h2>{{ displayTitle }}</h2>
              </div>
            </template>
          </div>
          <div v-if="!messageSelectionMode" class="header-streak">
            <span></span><span></span><span></span>
          </div>
          <div v-if="messageSelectionMode && (selectedPeer || selectedGroup)" class="message-selection-toolbar">
            <label title="自动加载整个会话后全部选中"><input type="checkbox" :disabled="loadingAllMessages" :checked="allMessagesSelected" @change="toggleAllMessages" /> {{ loadingAllMessages ? '加载中…' : (allMessagesSelected ? '已选 ' + selectedMessageIDs.length + ' 条' : '全选') }}</label>
            <button type="button" :disabled="!selectedMessageIDs.length || deletingMessages" @click="deleteSelectedMessages">删除选中</button>
            <button type="button" title="删除当前会话的全部消息" :disabled="!selectableMessages.length || deletingMessages" @click="deleteAllOwnMessages">全部删除</button>
          </div>
          <button
            v-if="selectedPeer || selectedGroup"
            class="message-manage-btn favorite-btn"
            type="button"
            @click="openFavorites"
          >★ 收藏</button>
          <button
            v-if="selectedPeer || selectedGroup"
            class="message-manage-btn"
            type="button"
            :class="{ active: messageSelectionMode }"
            @click="toggleMessageSelectionMode"
          >{{ messageSelectionMode ? '退出选择' : '管理消息' }}</button>
        </header>

        <div ref="messageBox" class="message-stage">
          <div v-if="!selectedPeer && !selectedGroup" class="empty-stage">
            <div class="mascot mascot-small">
              <img class="mascot-img" :src="mascotImage" alt="星讯吉祥物" />
            </div>
            <p>从左侧选择一位联系人</p>
            <span>聊天记录、搜索和文件都将在这里展开。</span>
          </div>

          <template v-else>
            <div class="conversation-marker">
              <span></span>
              <p>{{ selectedGroup ? selectedGroup.name : (selectedPeer.nickname || selectedPeer.username) }} 消息记录</p>
              <span></span>
            </div>

            <article
              v-for="message in messages"
              :key="message.id"
              :id="'msg-' + message.id"
              class="message-row"
              :class="{ mine: isMine(message) }"
            >
              <input
                v-if="messageSelectionMode"
                v-model="selectedMessageIDs"
                class="message-select-checkbox"
                type="checkbox"
                :value="message.id"
                :aria-label="'选择消息 #' + message.id"
              />
              <div v-if="!isMine(message)" class="avatar message-avatar" :style="avatarStyle(messageAuthor(message))">
                <img v-if="messageAuthor(message).avatar_url" :src="messageAuthor(message).avatar_url" :alt="messageAuthorName(message)" />
                <span v-else>{{ avatarText(messageAuthor(message)) }}</span>
              </div>
              <div class="message-content">
                <div class="message-meta">
                  <span>{{ messageAuthorName(message) }}</span>
                  <time>{{ formatTime(message.created_at) }}</time>
                </div>
                <div class="message-bubble">
                  <p v-if="message.recalled_at" class="recalled-message">{{ Number(message.recalled_by) === Number(me.id) ? '你撤回了一条消息' : '对方撤回了一条消息' }}</p>
                  <template v-else>
                  <button v-if="message.reply_to_id" type="button" class="reply-preview" @click="scrollToMessage(message.reply_to_id)">回复消息 #{{ message.reply_to_id }}</button>
                  <button v-if="message.content_type === 'red_packet'" type="button" class="red-packet-card" @click="handleRedPacketClick(message)">
                    <span class="red-packet-icon">🧧</span>
                    <span class="red-packet-body">
                      <b>{{ redPacketInfo(message).greeting }}</b>
                      <small>{{ hasGrabbed(message) ? '已领取' : '点击领取' }}</small>
                    </span>
                  </button>
                  <img v-else-if="isImage(message)" :src="message.object_url" :alt="message.file_name || '图片消息'" @click="previewImage(message)" />
                  <a v-else-if="isFile(message)" class="file-card" :href="message.object_url" target="_blank" rel="noreferrer">
                    <span class="file-icon">✧</span>
                    <span>
                      <b>{{ message.file_name || '附件' }}</b>
                      <small>{{ formatSize(message.file_size) || '点击打开文件' }}</small>
                    </span>
                    <i>↗</i>
                  </a>
                  <p v-else>{{ message.content }}</p>
                  </template>
                </div>
                <div v-if="!message.recalled_at" class="message-tools">
                  <button type="button" @click="startReply(message)">回复</button>
                  <button type="button" @click="toggleFavorite(message)">{{ message.is_favorite ? '取消收藏' : '收藏' }}</button>
                  <button v-if="isMine(message) && message.content_type === 'text'" type="button" @click="startEdit(message)">编辑</button>
                </div>
                <button v-if="isMine(message) && !message.recalled_at && recallSecondsLeft(message) > 0" type="button" class="recall-button" @click="recallMessage(message)">撤回（{{ recallSecondsLeft(message) }}秒）</button>
                <small v-if="message.edited_at" class="edited-label">已编辑</small>
              </div>
            </article>
          </template>
        </div>

        <form class="composer" @submit.prevent="sendMessage">
          <div v-if="replyTarget || editingMessageID" class="composer-context">
            <span>{{ editingMessageID ? '正在编辑消息' : ('回复消息 #' + replyTarget.id) }}</span>
            <button type="button" @click="editingMessageID = 0; cancelReply(); composer = ''">取消</button>
          </div>
          <p v-if="typingHint || aiTyping" class="typing-hint">{{ aiTyping ? 'AI 好友正在思考…' : typingHint }}</p>
          <div v-if="selectedFile" class="file-preview">
            <span>✧ {{ selectedFile.name }}</span>
            <button type="button" @click="selectedFile = null; fileInput.value = ''">×</button>
          </div>
          <textarea
            v-model="composer"
            :disabled="!selectedPeer && !selectedGroup"
            placeholder="写下想说的话…"
            rows="1"
            @input="sendTyping"
            @keydown.enter.exact.prevent="sendMessage"
          ></textarea>
          <div class="composer-actions">
            <button v-if="selectedGroup" type="button" class="red-packet-btn" title="发红包" @click="redPacketOpen = true">🧧 红包</button>
            <label class="attach-button" title="上传文件">
              <input ref="fileInput" type="file" accept="image/jpeg,image/png,image/gif,image/webp,application/pdf,text/plain" @change="pickFile" />
              <span>＋ 附件</span>
            </label>
            <button v-if="selectedFile" type="button" class="send-file-button" :disabled="loading" @click="uploadMedia">发送文件</button>
            <button type="submit" class="send-button" :disabled="(!selectedPeer && !selectedGroup) || !composer.trim()">{{ editingMessageID ? '保存编辑' : '发送' }} <b>↗</b></button>
          </div>
          <div v-if="loading && uploadProgress" class="upload-progress"><i :style="{ width: uploadProgress + '%' }"></i></div>
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
          <p>{{ selectedGroup ? '正在搜索当前群聊' : (selectedPeer ? '正在搜索当前会话' : '搜索你可查看的全部消息') }}</p>
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
          <p>{{ socketState === 'open' ? '你已接入实时消息网络。' : '正在尝试恢复实时消息连接。' }}</p>
          <div class="signal-line"><i :class="{ moving: socketState === 'open' }"></i></div>
        </div>

        <div class="privacy-note">
          <span>◌</span>
          <p>文件使用临时访问链接传递，聊天内容由服务端安全处理。</p>
        </div>
      </aside>
    </section>

    <Teleport to="body">
    <div v-if="groupModalOpen" class="group-modal-backdrop" @click.self="closeGroupModal">
      <form class="group-modal" @submit.prevent="createGroup">
        <div class="group-modal-head">
          <div>
            <p class="eyebrow">新建群聊</p>
            <h2>创建群聊</h2>
          </div>
          <button class="icon-button" type="button" aria-label="关闭" title="关闭" @click="closeGroupModal">×</button>
        </div>
        <label class="field-label">
          <span>群名称</span>
          <input v-model.trim="groupName" maxlength="64" autofocus placeholder="在此处输入群名称"/>
        </label>
        <div class="group-member-picker">
          <div class="picker-label">添加参与者</div>
          <label v-for="user in users" :key="user.id" class="picker-item">
            <input v-model="selectedMemberIDs" type="checkbox" :value="user.id" />
            <span class="avatar picker-avatar" :style="avatarStyle(user)">{{ avatarText(user) }}</span>
            <span>{{ user.nickname || user.username }}</span>
          </label>
          <p v-if="!users.length" class="empty-groups">暂无可添加的联系人</p>
        </div>
        <div class="group-modal-actions">
          <button class="secondary-action" type="button" :disabled="creatingGroup" @click="closeGroupModal">取消</button>
          <button class="primary-action" type="submit" :disabled="creatingGroup || !groupName.trim()">
            {{ creatingGroup ? '创建中...' : '创建群聊' }}
          </button>
        </div>
      </form>
    </div>

    <div v-if="imagePreviewURL" class="image-preview-backdrop" @click.self="imagePreviewURL = ''">
      <button type="button" class="image-preview-close" @click="imagePreviewURL = ''">关闭</button>
      <img :src="imagePreviewURL" alt="图片预览" />
    </div>

    <div v-if="settingsOpen" class="group-modal-backdrop" @click.self="closeSettings">
      <form class="group-modal settings-modal" @submit.prevent="saveSettings">
        <div class="group-modal-head">
          <div>
            <p class="eyebrow">PERSONAL SETTINGS</p>
            <h2>个人设置</h2>
          </div>
          <button class="icon-button" type="button" aria-label="关闭" title="关闭" @click="closeSettings">×</button>
        </div>

        <div class="settings-avatar-area">
          <div class="avatar avatar-settings" :style="avatarStyle(me)">
            <img v-if="settingsAvatarPreview" :src="settingsAvatarPreview" :alt="me.nickname || me.username" />
            <img v-else-if="me.avatar_url" :src="me.avatar_url" :alt="me.nickname || me.username" />
            <span v-else>{{ avatarText(me) }}</span>
          </div>
          <label class="settings-avatar-btn">
            <input ref="settingsAvatarInput" type="file" accept="image/*" @change="pickAvatarFile" />
            <span>更换头像</span>
          </label>
        </div>

        <label class="field-label">
          <span>昵称</span>
          <input v-model.trim="settingsNickname" maxlength="64" placeholder="输入新昵称" />
        </label>

        <div class="settings-divider"></div>
        <p class="settings-section-title">修改密码</p>

        <label class="field-label">
          <span>旧密码</span>
          <input v-model="settingsOldPassword" type="password" placeholder="输入旧密码" />
        </label>

        <label class="field-label">
          <span>新密码</span>
          <input v-model="settingsNewPassword" type="password" placeholder="至少 6 位" />
        </label>

        <div class="group-modal-actions">
          <button class="secondary-action" type="button" :disabled="savingSettings" @click="closeSettings">取消</button>
          <button class="primary-action settings-save-button" type="submit" :disabled="savingSettings">{{ savingSettings ? '保存中…' : '保存设置' }}</button>
        </div>
      </form>
    </div>

    <div v-if="groupManageOpen" class="group-modal-backdrop" @click.self="closeGroupManage">
      <div class="group-modal settings-modal group-settings-modal">
        <div class="group-modal-head">
          <div>
            <p class="eyebrow">GROUP SETTINGS</p>
            <h2>{{ selectedGroup?.name || '群管理' }}</h2>
          </div>
          <button class="icon-button" type="button" aria-label="关闭" title="关闭" @click="closeGroupManage">×</button>
        </div>

        <div class="settings-avatar-area">
          <div class="avatar avatar-settings group-avatar">
            <img v-if="groupManageAvatarPreview" :src="groupManageAvatarPreview" :alt="selectedGroup?.name" />
            <img v-else-if="selectedGroup?.avatar_url" :src="selectedGroup.avatar_url" :alt="selectedGroup?.name" />
            <span v-else>#</span>
          </div>
          <label class="settings-avatar-btn">
            <input type="file" accept="image/*" @change="pickGroupAvatar" />
            <span>更换群头像</span>
          </label>
          <button v-if="groupManageAvatarFile" class="primary-action" :disabled="groupManageLoading" @click="saveGroupAvatar" style="margin-top:0.4rem">
            {{ groupManageLoading ? '上传中…' : '保存头像' }}
          </button>
        </div>

        <div class="settings-divider"></div>
        <template v-if="isGroupManager()">
          <p class="settings-section-title">群公告</p>
          <div class="group-invite-row">
            <input v-model.trim="groupAnnouncementDraft" maxlength="500" placeholder="输入群公告" />
            <button type="button" :disabled="groupManageLoading" @click="saveGroupAnnouncement">保存</button>
          </div>
          <button type="button" class="group-control-btn" :disabled="groupManageLoading" @click="toggleAllMuted">{{ selectedGroup?.all_muted ? '解除全员禁言' : '开启全员禁言' }}</button>
          <div class="settings-divider"></div>
        </template>
        <p class="settings-section-title">参与者列表 ({{ groupMembers.length }})</p>

        <div class="group-member-list">
          <div v-for="member in groupMembers" :key="member.user_id" class="group-member-row">
            <span :class="{ 'group-owner-mark': member.role === 'owner' }">
              {{ member.role === 'owner' ? '群主' : '' }} #{{ member.user_id }}
            </span>
            <span class="member-management-actions" v-if="member.role !== 'owner' && isGroupManager()">
              <button class="kick-btn" @click="muteGroupMember(member)">{{ member.muted_until ? '解除禁言' : '禁言' }}</button>
              <button v-if="me.id === selectedGroup.owner_id" class="kick-btn" @click="setGroupMemberRole(member, member.role === 'admin' ? 'member' : 'admin')">{{ member.role === 'admin' ? '取消管理员' : '设为管理员' }}</button>
              <button class="kick-btn" @click="kickGroupMember(member.user_id)">移出</button>
            </span>
          </div>
        </div>

        <div class="settings-divider"></div>
        <p class="settings-section-title">邀请参与者</p>

        <div class="group-invite-row">
          <input v-model.trim="groupManageInviteQuery" placeholder="输入用户名或 ID" @keyup.enter="searchUserToInvite" />
          <button type="button" :disabled="groupManageLoading || !groupManageInviteQuery.trim()" @click="searchUserToInvite">搜索</button>
        </div>
        <div v-if="groupManageInviteResults.length" class="group-invite-results">
          <div v-for="user in groupManageInviteResults" :key="user.id" class="group-invite-item">
            <span>#{{ user.id }} {{ user.nickname || user.username }}</span>
            <button type="button" @click="inviteGroupMember(user)">邀请</button>
          </div>
        </div>

        <div class="danger-zone">
          <p>解散后，所有参与者、消息和待处理入群申请都会被永久删除。</p>
          <button type="button" class="danger-action" :disabled="groupManageLoading" @click="dissolveGroup">解散群聊</button>
        </div>
        <button v-if="me.id !== selectedGroup.owner_id" type="button" class="group-leave-btn" @click="leaveGroup">退出群聊</button>
      </div>
    </div>

    <div v-if="addFriendOpen" class="group-modal-backdrop" @click.self="closeAddFriendModal">
      <div class="group-modal add-friend-modal">
        <div class="group-modal-head">
          <div><h2>添加好友</h2></div>
          <button class="icon-button" type="button" title="关闭" @click="closeAddFriendModal">×</button>
        </div>
        <div class="group-invite-row">
          <input v-model.trim="addFriendQuery" placeholder="输入用户名 / 昵称 / ID" @keyup.enter="searchAddFriend" autofocus />
          <button type="button" :disabled="addFriendLoading || !addFriendQuery.trim()" @click="searchAddFriend">搜索</button>
        </div>
        <div v-if="addFriendResults.length" class="directory-results">
          <div v-for="user in addFriendResults" :key="user.id" class="directory-result">
            <span>#{{ user.id }} {{ user.nickname || user.username }}</span>
            <button type="button" @click="sendFriendRequest(user)">加好友</button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="joinGroupOpen" class="group-modal-backdrop" @click.self="closeJoinGroupModal">
      <div class="group-modal add-friend-modal">
        <div class="group-modal-head">
          <div><h2>加入群聊</h2></div>
          <button class="icon-button" type="button" title="关闭" @click="closeJoinGroupModal">×</button>
        </div>
        <div class="group-invite-row">
          <input v-model.trim="joinGroupQuery" placeholder="输入群名或 ID" @keyup.enter="searchJoinGroup" autofocus />
          <button type="button" :disabled="joinGroupLoading || !joinGroupQuery.trim()" @click="searchJoinGroup">搜索</button>
        </div>
        <div v-if="joinGroupResults.length" class="directory-results">
          <div v-for="group in joinGroupResults" :key="group.id" class="directory-result">
            <span>#{{ group.id }} {{ group.name }}</span>
            <button type="button" @click="requestGroupJoin(group)">申请加入</button>
          </div>
        </div>
      </div>
    </div>

    <div v-if="redeemOpen" class="group-modal-backdrop" @click.self="redeemOpen = false">
      <div class="group-modal red-packet-modal">
        <div class="group-modal-head">
          <div><h2>我的钱包</h2></div>
          <button class="icon-button" type="button" title="关闭" @click="redeemOpen = false">×</button>
        </div>
        <div class="wallet-balance">
          <span>当前余额</span>
          <strong>¥{{ cents(balance) }}</strong>
        </div>
        <input v-model.trim="redeemCode" placeholder="输入兑换码充值" @keyup.enter="redeem" autofocus />
        <div class="group-modal-actions">
          <button class="secondary-action" type="button" @click="redeemOpen = false">取消</button>
          <button class="primary-action" type="button" :disabled="redeeming || !redeemCode.trim()" @click="redeem">兑换</button>
        </div>
      </div>
    </div>

    <div v-if="redPacketOpen" class="group-modal-backdrop" @click.self="redPacketOpen = false">
      <form class="group-modal red-packet-modal" @submit.prevent="sendRedPacket">
        <div class="group-modal-head">
          <div><h2>发红包</h2></div>
          <button class="icon-button" type="button" title="关闭" @click="redPacketOpen = false">×</button>
        </div>
        <label class="red-packet-field"><span>金额（元）</span><input v-model="redPacketAmount" type="number" min="0.01" step="0.01" placeholder="0.00" /></label>
        <label class="red-packet-field"><span>个数</span><input v-model.number="redPacketCount" type="number" min="1" /></label>
        <label class="red-packet-lucky"><input v-model="redPacketLucky" type="checkbox" /> 拼手气红包（随机金额）</label>
        <input v-model="redPacketGreeting" class="red-packet-greeting" maxlength="50" placeholder="祝福语（可选）" />
        <div class="group-modal-actions">
          <button class="secondary-action" type="button" @click="redPacketOpen = false">取消</button>
          <button class="primary-action" type="submit" :disabled="sendingRedPacket">发红包</button>
        </div>
      </form>
    </div>

    <div v-if="grabbedShow" class="group-modal-backdrop" @click.self="grabbedShow = false">
      <div class="group-modal red-packet-result-modal">
        <span class="red-packet-big">🧧</span>
        <h2>恭喜抢到</h2>
        <strong class="grabbed-amount">¥{{ cents(grabbedAmount) }}</strong>
        <button class="primary-action" type="button" @click="grabbedShow = false">收下</button>
      </div>
    </div>

    <div v-if="redPacketDetailOpen" class="group-modal-backdrop" @click.self="redPacketDetailOpen = false">
      <div class="group-modal red-packet-detail-modal">
        <div class="group-modal-head">
          <div><h2>{{ redPacketDetail?.packet?.greeting || '红包详情' }}</h2></div>
          <button class="icon-button" type="button" title="关闭" @click="redPacketDetailOpen = false">×</button>
        </div>
        <div class="rp-detail-summary">
          <span>总金额 <b>¥{{ cents(redPacketDetail?.packet?.total_amount) }}</b></span>
          <span>已领 <b>{{ redPacketDetail?.receipts?.length || 0 }}</b>/{{ redPacketDetail?.packet?.total_count }} 个</span>
        </div>
        <div class="rp-detail-list">
          <div v-for="r in redPacketDetail?.receipts" :key="r.id" class="rp-detail-row">
            <span>{{ userNameByID(r.user_id) }}</span>
            <strong>¥{{ cents(r.amount) }}</strong>
          </div>
          <div v-if="!redPacketDetail?.receipts?.length" class="rp-detail-empty">还没有人领取</div>
        </div>
      </div>
    </div>

    <div v-if="favoritesOpen" class="group-modal-backdrop" @click.self="favoritesOpen = false">
      <div class="group-modal favorite-modal">
        <div class="group-modal-head">
          <div><h2>收藏的消息</h2></div>
          <button class="icon-button" type="button" title="关闭" @click="favoritesOpen = false">×</button>
        </div>
        <div v-if="loadingFavorites" class="favorite-empty">加载中…</div>
        <div v-else class="favorite-list">
          <button v-for="m in favoriteMessages" :key="m.id" class="favorite-item" @click="jumpToMessage(m)">
            <span class="favorite-item-author">{{ messageAuthorName(m) }}</span>
            <p>{{ m.content || m.file_name || '[媒体消息]' }}</p>
            <small>{{ formatTime(m.created_at) }}</small>
          </button>
          <div v-if="!favoriteMessages.length" class="favorite-empty">还没有收藏的消息</div>
        </div>
      </div>
    </div>
    </Teleport>
  </main>
</template>
