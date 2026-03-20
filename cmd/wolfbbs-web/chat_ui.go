package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

type chatChannelSnapshot struct {
	Name               string `json:"name"`
	Topic              string `json:"topic,omitempty"`
	Locked             bool   `json:"locked"`
	Joined             bool   `json:"joined"`
	OnlineCount        int    `json:"online_count"`
	LastMessageID      int64  `json:"last_message_id"`
	LastMessageAt      string `json:"last_message_at,omitempty"`
	LastMessageLabel   string `json:"last_message_label,omitempty"`
	LastMessageFrom    string `json:"last_message_from,omitempty"`
	LastMessagePreview string `json:"last_message_preview,omitempty"`
	RecentMessages     int    `json:"recent_messages"`
	Active             bool   `json:"active"`
}

type chatBootstrapPayload struct {
	CSRF           string                `json:"csrf"`
	CurrentChannel string                `json:"current_channel"`
	CurrentHandle  string                `json:"current_handle"`
	CanModerate    bool                  `json:"can_moderate"`
	Details        []chatChannelSnapshot `json:"details"`
	Joined         []string              `json:"joined"`
	Locked         []string              `json:"locked"`
}

func normalizeChatPageChannel(raw string) string {
	channel := strings.TrimSpace(raw)
	if channel == "" {
		return "#lobby"
	}
	if channel[0] != '#' {
		return "#" + channel
	}
	return channel
}

func chatActivityLabel(now, at time.Time) string {
	if at.IsZero() {
		return "quiet"
	}
	delta := now.Sub(at)
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return at.Local().Format("15:04")
	default:
		return at.Local().Format("Jan 2 15:04")
	}
}

func chatTopicForChannel(channel string, locked bool) string {
	channel = normalizeChatPageChannel(channel)
	base := "Shared live room for web, SSH, and IRC callers."
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "#lobby":
		base = "Main lobby for general chat, greetings, and quick social check-ins."
	case "#help":
		base = "Ask for help, onboarding tips, and operator nudges here."
	default:
		label := strings.TrimPrefix(channel, "#")
		label = strings.TrimSpace(strings.ReplaceAll(label, "-", " "))
		if label != "" {
			base = "Live room for " + label + " conversation across the board."
		}
	}
	if locked {
		return base + " Read-only for non-moderators while the room is locked."
	}
	return base
}

func (a *webApp) chatChannelSnapshots(handle, current string) []chatChannelSnapshot {
	current = normalizeChatPageChannel(current)
	channels := []string{"#lobby"}
	if a.chatSvc != nil {
		channels = a.chatSvc.ListChannels()
		if len(channels) == 0 {
			channels = []string{"#lobby"}
		}
	}
	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(channels)+1)
	for _, channel := range append(channels, current) {
		channel = normalizeChatPageChannel(channel)
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		ordered = append(ordered, channel)
	}
	now := time.Now().UTC()
	out := make([]chatChannelSnapshot, 0, len(ordered))
	for _, channel := range ordered {
		snapshot := chatChannelSnapshot{
			Name:   channel,
			Locked: a.isChannelLocked(channel),
			Joined: channel == "#lobby" || channel == current,
		}
		snapshot.Topic = chatTopicForChannel(channel, snapshot.Locked)
		if a.chatSvc != nil {
			snapshot.Joined = snapshot.Joined || a.chatSvc.IsInChannel(handle, channel)
			snapshot.OnlineCount = len(a.chatSvc.OnlineInChannel(channel))
			history := a.chatSvc.History(channel, 25)
			snapshot.RecentMessages = len(history)
			if len(history) > 0 {
				last := history[len(history)-1]
				snapshot.LastMessageID = last.ID
				snapshot.LastMessageAt = last.CreatedAt.UTC().Format(time.RFC3339)
				snapshot.LastMessageLabel = chatActivityLabel(now, last.CreatedAt.UTC())
				snapshot.LastMessageFrom = defaultIfBlank(last.From, "system")
				snapshot.LastMessagePreview = cleanOneLiner(last.Body, 78)
				snapshot.Active = snapshot.OnlineCount > 0 || now.Sub(last.CreatedAt.UTC()) < 24*time.Hour
			}
		}
		if snapshot.Joined {
			snapshot.Active = true
		}
		out = append(out, snapshot)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == current {
			return true
		}
		if out[j].Name == current {
			return false
		}
		if out[i].Joined != out[j].Joined {
			return out[i].Joined
		}
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		if out[i].OnlineCount != out[j].OnlineCount {
			return out[i].OnlineCount > out[j].OnlineCount
		}
		if out[i].LastMessageID != out[j].LastMessageID {
			return out[i].LastMessageID > out[j].LastMessageID
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (a *webApp) renderChatPage(r *http.Request, user *domain.User) string {
	csrf := ""
	if state, ok := currentSessionState(r, a); ok {
		csrf = state.csrf
	}
	canModerate := a.hasRole(user, roleModerator)
	currentChannel := normalizeChatPageChannel(r.URL.Query().Get("channel"))
	details := a.chatChannelSnapshots(user.Handle, currentChannel)
	joined := make([]string, 0, len(details))
	locked := make([]string, 0, len(details))
	for _, row := range details {
		if row.Joined {
			joined = append(joined, row.Name)
		}
		if row.Locked {
			locked = append(locked, row.Name)
		}
	}
	bootstrap, _ := json.Marshal(chatBootstrapPayload{
		CSRF:           csrf,
		CurrentChannel: currentChannel,
		CurrentHandle:  strings.ToLower(strings.TrimSpace(user.Handle)),
		CanModerate:    canModerate,
		Details:        details,
		Joined:         joined,
		Locked:         locked,
	})
	chatEmptyHelper := ""
	if a.chatSvc == nil || len(a.chatSvc.History("#lobby", 1)) == 0 {
		chatEmptyHelper = a.renderRoleAwareEmptyState(user, "chat")
	}
	modActions := ""
	if canModerate {
		modActions = `<article id="mod" class="wolfbbs-card wolfbbs-chat-side-card"><h2>Moderator Desk</h2><form id="modForm" action="/chat/moderation" method="POST" class="wolfbbs-stack" data-no-auto-busy="1">` + a.csrfHiddenInput(r) + `<label>Channel<input type="text" name="channel" value="` + htmlEscape(currentChannel) + `"></label><label>Target<input type="text" name="target" placeholder="target" required></label><label>Reason<input type="text" name="reason" placeholder="why this action is happening"></label><label>Duration (optional)<input type="text" name="duration" value="" placeholder="5m"></label><label>Action<select name="action"><option value="kick">Kick</option><option value="mute">Mute</option><option value="unmute">Unmute</option><option value="ban">Ban</option><option value="unban">Unban</option></select></label><button type="submit">Apply moderation</button></form><p class="wolfbbs-muted">Work from the channel you are actively reviewing so target, roster, and action history stay aligned.</p></article>`
	}
	return `<!doctype html>
<html>
<body>
	<h1>` + htmlEscape(a.siteDisplayName()) + ` Chat</h1>
	<p><a href="/boards">boards</a> | <a href="/today">today</a> | <a href="/attention">attention</a> | <a href="/bookmarks">bookmarks</a> | <a href="/mail">mail</a> | <a href="/doors">doors</a> | <a href="/clubhouse">clubhouse</a> | <a href="/help">help</a> | <a href="/logout">logout</a></p>
	` + pageMessageBlock(r) + `
	<div class="wolfbbs-chat-primer"><strong>` + htmlEscape(user.Handle) + `</strong> is on the live channel layer shared with IRC. Use the left rail like a Slack or AOL sidebar, keep the transcript centered, and treat the right rail as your channel desk.</div>
	` + chatEmptyHelper + `
	<section class="wolfbbs-chat-shell">
		<aside class="wolfbbs-chat-column wolfbbs-chat-rail">
			<article class="wolfbbs-card wolfbbs-chat-side-card">
				<h2>Joined Channels</h2>
				<p class="wolfbbs-muted">Your current channel set, shaped like a modern sidebar but still legible to IRC people.</p>
				<div id="chatJoinedRooms" class="wolfbbs-room-list"></div>
			</article>
			<article class="wolfbbs-card wolfbbs-chat-side-card">
				<h2>Active Channels</h2>
				<p class="wolfbbs-muted">Channels with live people or recent traffic bubble up here.</p>
				<div id="chatActiveRooms" class="wolfbbs-room-list"></div>
			</article>
			<article class="wolfbbs-card wolfbbs-chat-side-card">
				<h2>Open Or Create Channel</h2>
				<form id="customChannelForm" class="wolfbbs-stack">
					<label>Channel name<input type="text" id="customChannel" placeholder="#ansi-lab" autocomplete="off"></label>
					<button type="submit">Open Channel</button>
				</form>
				<p class="wolfbbs-muted">Use ` + "<code>#channel-name</code>" + ` or just type the name and the UI will normalize it.</p>
			</article>
		</aside>
		<section class="wolfbbs-chat-column wolfbbs-chat-stage">
			<article class="wolfbbs-card wolfbbs-chat-header-card">
				<div class="wolfbbs-chat-header-main">
					<div>
						<p class="wolfbbs-chat-kicker">Live channel</p>
						<h2 id="chatRoomTitle">` + htmlEscape(currentChannel) + `</h2>
						<p id="chatRoomSubline" class="wolfbbs-muted">Shared with IRC and web callers.</p>
					</div>
					<div class="wolfbbs-chat-header-stats">
						<span class="wolfbbs-chat-status-pill" data-state="idle" id="chatStatePill">idle</span>
						<span id="chatRoomLast">waiting</span>
						<span id="chatMessageCount">0 lines</span>
					</div>
				</div>
				<div class="wolfbbs-chat-toolbar">
					<label>Switch channel<select id="channelSelect"></select></label>
					<button type="button" id="chatCompactToggle">Compact View</button>
					<button type="button" id="chatReconnect">Reconnect Stream</button>
					<button type="button" id="chatLeaveRoom">Leave Channel</button>
				</div>
			</article>
			<article class="wolfbbs-card wolfbbs-chat-transcript-card">
				<div id="chat" class="wolfbbs-chat-pane" aria-live="polite"></div>
				<p id="chatStatus">Ready.</p>
			</article>
			<article class="wolfbbs-card wolfbbs-chat-composer-card">
				<form id="sendForm" class="wolfbbs-chat-composer-form">
					<label class="wolfbbs-chat-composer-field">Message<input type="text" id="message" autocomplete="off" placeholder="Say something in the channel..."></label>
					<button type="submit" id="sendButton">Send</button>
				</form>
				<p class="wolfbbs-muted">Enter sends. Ctrl+L clears the local transcript pane. Drafts stay per channel while you switch.</p>
			</article>
		</section>
		<aside class="wolfbbs-chat-column wolfbbs-chat-rail">
			<article class="wolfbbs-card wolfbbs-chat-side-card">
				<h2>Channel Desk</h2>
				<dl class="wolfbbs-meta-list"><div><dt>Current channel</dt><dd id="chatCurrentChannel">` + htmlEscape(currentChannel) + `</dd></div><div><dt>Mode</dt><dd id="chatMode">open</dd></div><div><dt>Online now</dt><dd id="onlineCount">0</dd></div><div><dt>Last sync</dt><dd id="chatLastSync">waiting</dd></div></dl>
				<div id="chatRoomDesk" class="wolfbbs-chat-room-desk"></div>
			</article>
			<article class="wolfbbs-card wolfbbs-chat-side-card">
				<h2>Who Is Here</h2>
				<div id="online">loading...</div>
				<p class="wolfbbs-muted">Roster entries reflect whoever is actively in this channel, including IRC callers.</p>
			</article>
			<article class="wolfbbs-card wolfbbs-chat-side-card">
				<h2>Fast Moves</h2>
				<div class="wolfbbs-action-grid"><a class="wolfbbs-action-card" href="/clubhouse"><strong>Clubhouse</strong><span>one-liners and the neighboring social layer</span></a><a class="wolfbbs-action-card" href="/attention"><strong>Attention</strong><span>direct follow-up and return hooks</span></a><a class="wolfbbs-action-card" href="/connect"><strong>Connect</strong><span>SSH, IRC, and terminal access help</span></a></div>
			</article>
			` + modActions + `
		</aside>
	</section>
	<script id="chatBootstrap" type="application/json">` + string(bootstrap) + `</script>
	<script>
		const bootstrapNode = document.getElementById('chatBootstrap');
		const chatBootstrap = bootstrapNode ? JSON.parse(bootstrapNode.textContent || '{}') : {};
		const csrf = chatBootstrap.csrf || '';
		const canModerate = !!chatBootstrap.can_moderate;
		const currentHandle = String(chatBootstrap.current_handle || '').toLowerCase();
		const storageKeys = { drafts: 'wolfbbs.chat.drafts', lastRead: 'wolfbbs.chat.lastRead', compact: 'wolfbbs.chat.compact' };
		const streamState = {
			es: null,
			channel: chatBootstrap.current_channel || '#lobby',
			lastSeenId: 0,
			reconnectTimer: null,
			locked: new Set(chatBootstrap.locked || []),
			joined: new Set(chatBootstrap.joined || []),
			summaries: new Map(),
			statusHoldUntil: 0,
			statusHoldTimer: null,
			lastRead: loadJSON(storageKeys.lastRead),
			drafts: loadJSON(storageKeys.drafts),
			compact: loadFlag(storageKeys.compact),
		};
		let roomRefreshTimer = null;
		let roomRefreshInFlight = false;
		let chatComposerPrimed = false;

		function loadJSON(key) {
			try {
				const raw = window.localStorage.getItem(key);
				return raw ? JSON.parse(raw) : {};
			} catch (err) {
				return {};
			}
		}

		function saveJSON(key, value) {
			try {
				window.localStorage.setItem(key, JSON.stringify(value || {}));
			} catch (err) {}
		}

		function loadFlag(key) {
			try {
				return window.localStorage.getItem(key) === '1';
			} catch (err) {
				return false;
			}
		}

		function saveFlag(key, value) {
			try {
				window.localStorage.setItem(key, value ? '1' : '0');
			} catch (err) {}
		}

		function msgValue(m, primary, legacy, fallback) {
			if (m && m[primary] !== undefined && m[primary] !== null && m[primary] !== '') return m[primary];
			if (m && legacy && m[legacy] !== undefined && m[legacy] !== null && m[legacy] !== '') return m[legacy];
			return fallback;
		}

		function presenceValue(m, primary, legacy, fallback) {
			if (m && m[primary] !== undefined && m[primary] !== null && m[primary] !== '') return m[primary];
			if (m && legacy && m[legacy] !== undefined && m[legacy] !== null && m[legacy] !== '') return m[legacy];
			return fallback;
		}

		function normalizeIncomingMessage(raw) {
			let m = raw;
			if (!m) return null;
			if (typeof m === 'string') return { id: 0, from: 'system', body: m, created_at: '--:--:--', channel: streamState.channel };
			if (typeof m.message === 'object' && m.message) m = m.message;
			if (typeof m.data === 'object' && m.data) m = m.data;
			if (typeof m.payload === 'object' && m.payload) m = m.payload;
			const from = msgValue(m, 'from', 'From', 'system');
			const body = msgValue(m, 'body', 'Body', msgValue(m, 'message', 'Message', ''));
			const createdAt = msgValue(m, 'created_at', 'CreatedAt', '--:--:--');
			const channel = msgValue(m, 'channel', 'Channel', streamState.channel);
			const id = Number(msgValue(m, 'id', 'ID', 0)) || 0;
			if (!from && !body) return null;
			return { id: id, from: from, body: body, created_at: createdAt, channel: channel };
		}

		function normalizeChannelName(value) {
			const raw = String(value || '').trim();
			if (!raw) return '';
			return raw[0] === '#' ? raw : ('#' + raw);
		}

		function isNearBottom(box) {
			return (box.scrollTop + box.clientHeight) >= (box.scrollHeight - 40);
		}

		function liveStatusText() {
			const readOnly = streamState.locked.has(streamState.channel) && !canModerate;
			if (readOnly) return { msg: 'Live on ' + streamState.channel + ' (read-only)', state: 'warn' };
			return { msg: 'Live on ' + streamState.channel, state: 'live' };
		}

		function setStatus(msg, state, opts) {
			const options = opts || {};
			const now = Date.now();
			const holdActive = streamState.statusHoldUntil > now;
			if (holdActive && !options.force) {
				const resolved = state || (String(msg || '').toLowerCase().includes('failed') ? 'error' : 'ready');
				if (resolved === 'live' || resolved === 'ready') return;
			}
			const el = document.getElementById('chatStatus');
			if (el) el.textContent = msg;
			const pill = document.getElementById('chatStatePill');
			if (pill) {
				const resolved = state || (String(msg || '').toLowerCase().includes('failed') ? 'error' : 'ready');
				pill.dataset.state = resolved;
				pill.textContent = resolved;
			}
			if (streamState.statusHoldTimer && options.clearHold) {
				window.clearTimeout(streamState.statusHoldTimer);
				streamState.statusHoldTimer = null;
				streamState.statusHoldUntil = 0;
			}
			if (options.stickyMs) {
				streamState.statusHoldUntil = now + options.stickyMs;
				if (streamState.statusHoldTimer) window.clearTimeout(streamState.statusHoldTimer);
				streamState.statusHoldTimer = window.setTimeout(() => {
					streamState.statusHoldUntil = 0;
					streamState.statusHoldTimer = null;
					const live = liveStatusText();
					setStatus(live.msg, live.state, { force: true });
				}, options.stickyMs);
			}
		}

		function updateMessageCount() {
			const box = document.getElementById('chat');
			const counter = document.getElementById('chatMessageCount');
			if (!box || !counter) return;
			const count = box.querySelectorAll('.wolfbbs-chat-line').length;
			counter.textContent = count + (count === 1 ? ' line' : ' lines');
		}

		function ensureSummary(summary) {
			if (!summary || !summary.name) return;
			const current = streamState.summaries.get(summary.name) || {};
			streamState.summaries.set(summary.name, Object.assign({}, current, summary));
		}

		function seedSummaries(details) {
			(details || []).forEach((row) => ensureSummary(row));
		}

		function sortSummaries(items) {
			return items.sort((a, b) => {
				if (a.name === streamState.channel) return -1;
				if (b.name === streamState.channel) return 1;
				if (!!a.joined !== !!b.joined) return a.joined ? -1 : 1;
				if (!!a.active !== !!b.active) return a.active ? -1 : 1;
				if ((a.online_count || 0) !== (b.online_count || 0)) return (b.online_count || 0) - (a.online_count || 0);
				if ((a.last_message_id || 0) !== (b.last_message_id || 0)) return (b.last_message_id || 0) - (a.last_message_id || 0);
				return String(a.name || '').localeCompare(String(b.name || ''));
			});
		}

		function renderRoomList(targetId, items, emptyText) {
			const wrap = document.getElementById(targetId);
			if (!wrap) return;
			wrap.innerHTML = '';
			if (!items.length) {
				const empty = document.createElement('p');
				empty.className = 'wolfbbs-muted';
				empty.textContent = emptyText;
				wrap.appendChild(empty);
				return;
			}
			items.forEach((row) => {
				const card = document.createElement('article');
				card.className = 'wolfbbs-room-card';
				if (row.name === streamState.channel) card.classList.add('active');
				if (row.locked) card.classList.add('locked');
				if ((row.last_message_id || 0) > Number(streamState.lastRead[row.name] || 0) && row.name !== streamState.channel) card.classList.add('unread');
				const head = document.createElement('div');
				head.className = 'wolfbbs-room-card-head';
				const trigger = document.createElement('button');
				trigger.type = 'button';
				trigger.className = 'wolfbbs-room-trigger';
				trigger.textContent = row.name;
				trigger.addEventListener('click', async () => { await openRoom(row.name, { join: true, focusComposer: true }); });
				head.appendChild(trigger);
				const badges = document.createElement('div');
				badges.className = 'wolfbbs-room-card-badges';
				if (row.locked) badges.appendChild(makeBadge('locked'));
				if ((row.online_count || 0) > 0) badges.appendChild(makeBadge(String(row.online_count) + ' live'));
				if ((row.last_message_id || 0) > Number(streamState.lastRead[row.name] || 0) && row.name !== streamState.channel) badges.appendChild(makeBadge('new'));
				head.appendChild(badges);
				card.appendChild(head);
				const meta = document.createElement('div');
				meta.className = 'wolfbbs-room-card-meta';
				meta.innerHTML = '<span>' + escapeHTML(row.last_message_label || 'quiet') + '</span><span>' + escapeHTML(row.last_message_from || 'system') + '</span>';
				card.appendChild(meta);
				const preview = document.createElement('p');
				preview.className = 'wolfbbs-room-card-preview';
				preview.textContent = row.last_message_preview || 'No traffic yet.';
				card.appendChild(preview);
				if (row.joined && row.name !== '#lobby') {
					const leave = document.createElement('button');
					leave.type = 'button';
					leave.className = 'wolfbbs-room-leave';
					leave.textContent = 'Leave';
					leave.addEventListener('click', async (evt) => {
						evt.stopPropagation();
						await leaveRoom(row.name);
					});
					card.appendChild(leave);
				}
				wrap.appendChild(card);
			});
		}

		function makeBadge(text) {
			const badge = document.createElement('span');
			badge.className = 'wolfbbs-room-badge';
			badge.textContent = text;
			return badge;
		}

		function renderRoomLists() {
			const all = sortSummaries(Array.from(streamState.summaries.values()));
			const joined = all.filter((row) => row.joined || row.name === streamState.channel || row.name === '#lobby');
			const active = all.filter((row) => !joined.some((joinedRow) => joinedRow.name === row.name) && (row.active || (row.online_count || 0) > 0 || (row.last_message_id || 0) > 0)).slice(0, 12);
			renderRoomList('chatJoinedRooms', joined, 'No joined channels yet. Open one from the channel box below.');
			renderRoomList('chatActiveRooms', active, 'No other active channels yet.');
			refreshRoomDesk();
		}

		function refreshRoomDesk() {
			const room = streamState.summaries.get(streamState.channel) || { name: streamState.channel };
			document.getElementById('chatCurrentChannel').textContent = streamState.channel;
			document.getElementById('chatRoomTitle').textContent = streamState.channel;
			document.getElementById('chatRoomSubline').textContent = (room.topic || 'Shared live room for callers.') + ' • ' + ((room.online_count || 0) > 0 ? String(room.online_count) + ' people live' : 'quiet roster');
			document.getElementById('chatRoomLast').textContent = room.last_message_label || 'waiting';
			const desk = document.getElementById('chatRoomDesk');
			if (desk) {
				desk.innerHTML = '';
				const rows = [
					['Topic', room.topic || 'Shared live room for callers.'],
					['Latest', room.last_message_preview || 'No traffic yet.'],
					['From', room.last_message_from || 'system'],
					['Messages', String(room.last_message_id || 0)],
					['Membership', room.joined ? 'joined' : 'watching'],
				];
				rows.forEach((pair) => {
					const block = document.createElement('div');
					block.className = 'wolfbbs-chat-room-desk-row';
					const dt = document.createElement('strong');
					dt.textContent = pair[0];
					const dd = document.createElement('span');
					dd.textContent = pair[1];
					block.appendChild(dt);
					block.appendChild(dd);
					desk.appendChild(block);
				});
			}
			document.getElementById('chatLeaveRoom').disabled = !room.joined || streamState.channel === '#lobby';
		}

		function applyCompactMode() {
			const shell = document.querySelector('.wolfbbs-chat-shell');
			if (shell) shell.classList.toggle('compact', !!streamState.compact);
			const button = document.getElementById('chatCompactToggle');
			if (button) button.textContent = streamState.compact ? 'Standard View' : 'Compact View';
		}

		function updateComposerState() {
			const message = document.getElementById('message');
			const sendButton = document.getElementById('sendButton');
			const locked = streamState.locked.has(streamState.channel);
			const readOnly = locked && !canModerate;
			message.disabled = readOnly;
			sendButton.disabled = readOnly;
			document.getElementById('chatMode').textContent = readOnly ? 'read-only' : (locked ? 'locked (mod write)' : 'open');
			message.placeholder = readOnly ? 'This channel is locked for non-moderators.' : 'Say something in the channel...';
			const modChannel = document.querySelector('#modForm input[name="channel"]');
			if (modChannel) modChannel.value = streamState.channel;
			refreshRoomDesk();
		}

		function noteSync() {
			document.getElementById('chatLastSync').textContent = new Date().toLocaleTimeString();
		}

		function renderTranscriptEmpty() {
			const box = document.getElementById('chat');
			box.innerHTML = '';
			const empty = document.createElement('div');
			empty.className = 'wolfbbs-chat-empty';
			empty.textContent = 'No lines in ' + streamState.channel + ' yet. Say hello and wake the channel up.';
			box.appendChild(empty);
			updateMessageCount();
		}

		function insertUnreadMarker() {
			const box = document.getElementById('chat');
			if (!box || box.querySelector('.wolfbbs-chat-separator')) return;
			const marker = document.createElement('div');
			marker.className = 'wolfbbs-chat-separator';
			marker.textContent = 'New since your last visit';
			box.appendChild(marker);
		}

		function appendMessage(m) {
			const row = normalizeIncomingMessage(m);
			if (!row) return;
			const box = document.getElementById('chat');
			const stick = isNearBottom(box);
			const isEmpty = box.querySelector('.wolfbbs-chat-empty');
			if (isEmpty) isEmpty.remove();
			const line = document.createElement('article');
			line.className = 'wolfbbs-chat-line';
			const fromValue = String(msgValue(row, 'from', 'From', 'system') || 'system');
			const bodyText = String(msgValue(row, 'body', 'Body', ''));
			const fromLower = fromValue.toLowerCase();
			if (fromLower === currentHandle) line.classList.add('wolfbbs-chat-line-self');
			if (fromLower === 'system' || fromLower === 'server') line.classList.add('wolfbbs-chat-line-system');
			if (currentHandle && bodyText.toLowerCase().includes('@' + currentHandle)) line.classList.add('wolfbbs-chat-line-mention');
			const avatar = document.createElement('div');
			avatar.className = 'wolfbbs-chat-line-avatar';
			avatar.textContent = (fromValue[0] || '#').toUpperCase();
			const bubble = document.createElement('div');
			bubble.className = 'wolfbbs-chat-line-bubble';
			const head = document.createElement('div');
			head.className = 'wolfbbs-chat-line-head';
			const author = document.createElement('strong');
			author.textContent = fromValue;
			const meta = document.createElement('span');
			meta.className = 'wolfbbs-chat-line-meta';
			meta.textContent = '[' + msgValue(row, 'created_at', 'CreatedAt', '--:--:--') + ']';
			head.appendChild(author);
			head.appendChild(meta);
			const body = document.createElement('div');
			body.className = 'wolfbbs-chat-line-body';
			body.textContent = bodyText;
			bubble.appendChild(head);
			bubble.appendChild(body);
			line.appendChild(avatar);
			line.appendChild(bubble);
			box.appendChild(line);
			const id = Number(msgValue(row, 'id', 'ID', 0)) || 0;
			if (id > streamState.lastSeenId) streamState.lastSeenId = id;
			rememberRead(streamState.channel, streamState.lastSeenId);
			updateSummaryFromMessage(row);
			if (stick) box.scrollTop = box.scrollHeight;
			updateMessageCount();
		}

		function updateSummaryFromMessage(row) {
			const channel = normalizeChannelName(msgValue(row, 'channel', 'Channel', streamState.channel));
			const current = streamState.summaries.get(channel) || { name: channel };
			current.last_message_id = Number(msgValue(row, 'id', 'ID', 0)) || current.last_message_id || 0;
			current.last_message_from = String(msgValue(row, 'from', 'From', 'system') || 'system');
			current.last_message_preview = String(msgValue(row, 'body', 'Body', '') || '');
			current.last_message_label = 'just now';
			current.active = true;
			streamState.summaries.set(channel, current);
			renderRoomLists();
		}

		function rememberDraft() {
			const field = document.getElementById('message');
			streamState.drafts[streamState.channel] = field.value || '';
			saveJSON(storageKeys.drafts, streamState.drafts);
		}

		function restoreDraft() {
			const field = document.getElementById('message');
			field.value = streamState.drafts[streamState.channel] || '';
		}

		function rememberRead(channel, id) {
			if (!channel || !id) return;
			streamState.lastRead[channel] = id;
			saveJSON(storageKeys.lastRead, streamState.lastRead);
			renderRoomLists();
		}

		function renderPresenceList(rows) {
			const online = document.getElementById('online');
			const list = Array.isArray(rows) ? rows : [];
			if (!list.length) {
				online.textContent = 'No one is active in this channel right now.';
				document.getElementById('onlineCount').textContent = '0';
				return;
			}
			const wrap = document.createElement('div');
			wrap.className = 'wolfbbs-presence-list';
			list.forEach((row) => {
				const card = document.createElement('div');
				card.className = 'wolfbbs-presence-card';
				const nickValue = String(presenceValue(row, 'nick', 'Nick', 'caller'));
				const nick = document.createElement('strong');
				nick.textContent = nickValue;
				card.appendChild(nick);
				const meta1 = document.createElement('span');
				meta1.textContent = String(presenceValue(row, 'area', 'Area', streamState.channel)) + ' • ' + String(presenceValue(row, 'node', 'Node', 'web'));
				card.appendChild(meta1);
				const meta2 = document.createElement('span');
				meta2.textContent = 'idle ' + String(presenceValue(row, 'idle_sec', 'IdleSec', 0)) + 's';
				card.appendChild(meta2);
				const actions = document.createElement('div');
				actions.className = 'wolfbbs-inline-actions';
				const mail = document.createElement('a');
				mail.href = '/mail?to=' + encodeURIComponent(nickValue) + '&subject=' + encodeURIComponent('Follow-up from ' + streamState.channel) + '&body=' + encodeURIComponent('Continuing from ' + streamState.channel + '.\n\n');
				mail.textContent = 'mail';
				actions.appendChild(mail);
				const page = document.createElement('a');
				page.href = '/directory?handle=' + encodeURIComponent(nickValue) + '#page-desk';
				page.textContent = 'page';
				actions.appendChild(page);
				card.appendChild(actions);
				wrap.appendChild(card);
			});
			online.innerHTML = '';
			online.appendChild(wrap);
			document.getElementById('onlineCount').textContent = String(list.length);
			const room = streamState.summaries.get(streamState.channel) || { name: streamState.channel };
			room.online_count = list.length;
			streamState.summaries.set(streamState.channel, room);
			renderRoomLists();
		}

		function escapeHTML(value) {
			return String(value || '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
		}

		async function loadChannels(opts) {
			const options = opts || {};
			const res = await fetch('/chat/channels?current=' + encodeURIComponent(streamState.channel), { credentials: 'same-origin' });
			if (!res.ok) {
				setStatus('Failed to load channels.', 'error');
				return;
			}
			const payload = await res.json();
			streamState.locked = new Set(payload.locked || []);
			streamState.joined = new Set(payload.joined || []);
			seedSummaries(payload.details || []);
			const select = document.getElementById('channelSelect');
			select.innerHTML = '';
			sortSummaries(Array.from(streamState.summaries.values())).forEach((room) => {
				const option = document.createElement('option');
				option.value = room.name;
				option.textContent = room.locked ? (room.name + ' [locked]') : room.name;
				select.appendChild(option);
			});
			if (!Array.from(select.options).some((option) => option.value === streamState.channel)) {
				const fallback = document.createElement('option');
				fallback.value = streamState.channel;
				fallback.textContent = streamState.channel;
				select.appendChild(fallback);
			}
			select.value = streamState.channel;
			renderRoomLists();
			updateComposerState();
			if (!options.quiet) setStatus('Channels updated.', 'ready');
		}

		async function loadHistory() {
			const res = await fetch('/chat/history?channel=' + encodeURIComponent(streamState.channel) + '&limit=100', { credentials: 'same-origin' });
			if (!res.ok) {
				setStatus('Could not load history for ' + streamState.channel, 'error');
				return;
			}
			const payload = await res.json();
			const box = document.getElementById('chat');
			box.innerHTML = '';
			streamState.lastSeenId = 0;
			const messages = payload.messages || [];
			const unreadBaseline = Number(streamState.lastRead[streamState.channel] || 0);
			let unreadMarkerShown = unreadBaseline <= 0;
			if (!messages.length) {
				renderTranscriptEmpty();
			} else {
				messages.forEach((message) => {
					const row = normalizeIncomingMessage(message);
					if (!unreadMarkerShown && row && (Number(row.id || 0) > unreadBaseline)) {
						insertUnreadMarker();
						unreadMarkerShown = true;
					}
					appendMessage(message);
				});
				box.scrollTop = box.scrollHeight;
			}
			if (payload.last_id) streamState.lastSeenId = Number(payload.last_id) || streamState.lastSeenId;
			rememberRead(streamState.channel, streamState.lastSeenId);
			renderPresenceList(payload.online || []);
			noteSync();
			updateMessageCount();
		}

		async function loadOnline() {
			const res = await fetch('/chat/online?channel=' + encodeURIComponent(streamState.channel), { credentials: 'same-origin' });
			if (!res.ok) {
				setStatus('Could not load channel roster.', 'error');
				return;
			}
			const payload = await res.json();
			renderPresenceList(payload.presence || []);
			noteSync();
		}

		function scheduleRoomRefresh(delay) {
			if (roomRefreshTimer) window.clearTimeout(roomRefreshTimer);
			roomRefreshTimer = window.setTimeout(async function () {
				if (document.hidden) {
					scheduleRoomRefresh(12000);
					return;
				}
				if (roomRefreshInFlight) {
					scheduleRoomRefresh(4000);
					return;
				}
				roomRefreshInFlight = true;
				try {
					await loadChannels({ quiet: true });
					await loadOnline();
				} finally {
					roomRefreshInFlight = false;
					scheduleRoomRefresh(document.hidden ? 12000 : 6000);
				}
			}, Math.max(600, delay || 6000));
		}

		function stopStream() {
			if (streamState.reconnectTimer) {
				window.clearTimeout(streamState.reconnectTimer);
				streamState.reconnectTimer = null;
			}
			if (roomRefreshTimer) {
				window.clearTimeout(roomRefreshTimer);
				roomRefreshTimer = null;
			}
			if (streamState.es) {
				streamState.es.close();
				streamState.es = null;
			}
		}

		function watch() {
			if (streamState.es) streamState.es.close();
			const es = new EventSource('/chat/stream?channel=' + encodeURIComponent(streamState.channel) + '&after_id=' + encodeURIComponent(String(streamState.lastSeenId || 0)));
			streamState.es = es;
			es.onopen = function () { const live = liveStatusText(); setStatus(live.msg, live.state); };
			es.onmessage = function (evt) {
				let msg = null;
				try { msg = JSON.parse(evt.data); } catch (err) {
					setStatus('Malformed stream event ignored.', 'warn');
					return;
				}
				appendMessage(msg);
				noteSync();
				const live = liveStatusText();
				setStatus(live.msg, live.state);
			};
			es.onerror = function () {
				es.close();
				setStatus('Realtime disconnected; retrying...', 'warn');
				streamState.reconnectTimer = window.setTimeout(watch, 1200);
			};
		}

		async function joinRoom(channel) {
			const res = await fetch('/chat/join', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
				body: JSON.stringify({ channel: channel }),
			});
			if (!res.ok) {
				const body = await res.text();
				setStatus('Open channel failed: ' + body, 'error');
				return false;
			}
			streamState.joined.add(channel);
			const room = streamState.summaries.get(channel) || { name: channel };
			room.joined = true;
			streamState.summaries.set(channel, room);
			return true;
		}

		async function leaveRoom(channel) {
			if (!channel || channel === '#lobby') return;
			const res = await fetch('/chat/leave', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
				body: JSON.stringify({ channel: channel }),
			});
			if (!res.ok) {
				setStatus('Leave channel failed.', 'error');
				return;
			}
			streamState.joined.delete(channel);
			const room = streamState.summaries.get(channel);
			if (room) room.joined = false;
			if (streamState.channel === channel) {
				streamState.channel = '#lobby';
				await openRoom('#lobby', { join: false, focusComposer: false });
			} else {
				renderRoomLists();
			}
		}

		function focusChatComposer() {
			if (!chatComposerPrimed) return;
			document.getElementById('message').focus();
		}

		async function openRoom(next, opts) {
			const options = opts || {};
			const channel = normalizeChannelName(next);
			if (!channel) return;
			rememberDraft();
			if (options.join !== false && !streamState.joined.has(channel) && !streamState.locked.has(channel)) {
				const ok = await joinRoom(channel);
				if (!ok) return;
			}
			streamState.channel = channel;
			await loadChannels({ quiet: true });
			await loadHistory();
			await loadOnline();
			watch();
			restoreDraft();
			updateComposerState();
			const live = liveStatusText();
			setStatus(live.msg, live.state);
			if (options.focusComposer) {
				chatComposerPrimed = true;
				focusChatComposer();
			}
		}

		document.getElementById('channelSelect').addEventListener('change', async function (evt) {
			await openRoom(evt.target.value, { join: true, focusComposer: true });
		});

		document.getElementById('customChannelForm').addEventListener('submit', async function (evt) {
			evt.preventDefault();
			const field = document.getElementById('customChannel');
			const channel = normalizeChannelName(field.value);
			if (!channel) return;
			field.value = '';
			await openRoom(channel, { join: true, focusComposer: true });
		});

		document.getElementById('chatReconnect').addEventListener('click', async function () {
			await loadChannels({ quiet: true });
			await openRoom(streamState.channel, { join: false, focusComposer: chatComposerPrimed });
		});

		document.getElementById('chatCompactToggle').addEventListener('click', function () {
			streamState.compact = !streamState.compact;
			saveFlag(storageKeys.compact, streamState.compact);
			applyCompactMode();
		});

		document.getElementById('chatLeaveRoom').addEventListener('click', async function () {
			await leaveRoom(streamState.channel);
		});

		document.getElementById('sendForm').addEventListener('submit', async function (evt) {
			evt.preventDefault();
			const field = document.getElementById('message');
			const message = field.value.trim();
			if (!message) return;
			setStatus('Sending message...', 'working');
			const sendRes = await fetch('/chat/send', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
				body: JSON.stringify({ channel: streamState.channel, message: message }),
			});
			if (!sendRes.ok) {
				const body = await sendRes.text();
				setStatus('Send failed: ' + body, 'error');
				return;
			}
			field.value = '';
			streamState.drafts[streamState.channel] = '';
			saveJSON(storageKeys.drafts, streamState.drafts);
			chatComposerPrimed = true;
			focusChatComposer();
			setStatus('Sent to ' + streamState.channel, 'live');
		});

		const messageField = document.getElementById('message');
		messageField.addEventListener('focus', function () { chatComposerPrimed = true; });
		messageField.addEventListener('pointerdown', function () { chatComposerPrimed = true; });
		messageField.addEventListener('input', function () { rememberDraft(); });
		messageField.addEventListener('keydown', function (evt) {
			if (evt.key === 'l' && evt.ctrlKey) {
				evt.preventDefault();
				renderTranscriptEmpty();
				setStatus('Chat pane cleared.', 'ready');
			}
		});

		const modForm = document.getElementById('modForm');
		if (modForm) {
			modForm.addEventListener('submit', async function (evt) {
				evt.preventDefault();
				const form = new FormData(modForm);
				const payload = {
					channel: normalizeChannelName(form.get('channel')),
					action: String(form.get('action') || '').trim(),
					target: String(form.get('target') || '').trim(),
					reason: String(form.get('reason') || '').trim(),
					duration: String(form.get('duration') || '').trim(),
				};
				const res = await fetch('/chat/moderation', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
					body: JSON.stringify(payload),
				});
				if (!res.ok) {
					setStatus('Moderation failed.', 'error');
					return;
				}
				setStatus('Moderation applied to ' + payload.channel, 'ready', { stickyMs: 2500 });
				if (payload.channel === streamState.channel) await loadOnline();
				await loadChannels({ quiet: true });
			});
		}

		window.addEventListener('load', async function () {
			seedSummaries(chatBootstrap.details || []);
			applyCompactMode();
			renderRoomLists();
			await loadChannels({ quiet: true });
			await openRoom(streamState.channel, { join: false, focusComposer: false });
			scheduleRoomRefresh(6000);
		});

		document.addEventListener('visibilitychange', function () {
			if (!document.hidden) {
				loadChannels({ quiet: true }).finally(function () {
					scheduleRoomRefresh(3000);
				});
				return;
			}
			scheduleRoomRefresh(12000);
		});

		window.addEventListener('beforeunload', function () {
			rememberDraft();
			stopStream();
		});
	</script>
</body>
</html>`
}
