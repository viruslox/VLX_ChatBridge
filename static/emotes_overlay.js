const wsPath = (window.VLX_CONFIG && window.VLX_CONFIG.WEBSOCKET_PATH) || '/vlxrobot/ws';
const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const host = window.location.host;

function connect() {
    const socket = new WebSocket(`${protocol}//${host}${wsPath}`);

    socket.onopen = () => console.log("Emote Wall Connected.");

    socket.onclose = () => {
        console.warn("Disconnected. Reconnecting...");
        setTimeout(connect, 3000);
    };

    socket.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);

            if (data.type === 'emote_wall') {
                if (data.emotes) {
                    spawnEmotes(data.emotes);
                }
                return;
            }

            if (data.type === 'chat_username') {
                if (data.username) {
                    createUsernameElement(data.username, data.color);
                }
                return;
            }
        } catch (e) {
            console.error(e);
        }
    };
}

function spawnEmotes(urls) {
    urls.forEach((url, index) => {
        // timerly spacing each emotes
        setTimeout(() => {
            createEmoteElement(url);
        }, index * 100);
    });
}

function createEmoteElement(url) {
    const img = document.createElement('img');
    img.src = url;
    img.classList.add('emote');

    // Random orizzontal position (0% - 90% width)
    const leftPos = Math.random() * 90;
    img.style.left = leftPos + 'vw';

    // Random animation time (4s to 8s)
    const duration = 4 + Math.random() * 4;
    img.style.animationDuration = duration + 's';

    // Random dimension
    const scale = 0.5 + Math.random() * 0.8; // ( 50% to 130%)
    img.style.transform = `scale(${scale})`;

    document.body.appendChild(img);

    // remove element after animation
    setTimeout(() => {
        img.remove();
    }, duration * 1000);
}

// createUsernameElement floats a first-time chatter's username using the same
// floatUp motion as emotes. Color comes from the user's chat color (or the
// Twitch default computed server-side).
function createUsernameElement(username, color) {
    const el = document.createElement('div');
    el.classList.add('chat-username');
    el.textContent = username;

    if (color) {
        el.style.color = color;
    }

    // Random horizontal position (0% - 80% width; a bit tighter than emotes
    // since text is wider than a 112px emote).
    const leftPos = Math.random() * 80;
    el.style.left = leftPos + 'vw';

    // Same duration band as emotes (4s to 8s) so they intermix naturally.
    const duration = 4 + Math.random() * 4;
    el.style.animationDuration = duration + 's';

    document.body.appendChild(el);

    setTimeout(() => {
        el.remove();
    }, duration * 1000);
}

connect();
