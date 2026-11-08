import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';

// drag.ts needs only a few browsing-context members: listeners on window,
// the computed --wails-draggable of the event target, and the native bridge.
const listeners = new Map();
const posted = [];

class TestHTMLElement {
    clientWidth = 100;
    clientHeight = 100;
}

class TestMouseEvent {
    constructor(type, init = {}) {
        this.type = type;
        this.button = init.button ?? 0;
        this.buttons = init.buttons ?? 0;
        this.detail = init.detail ?? 0;
        this.target = init.target ?? null;
        this.offsetX = init.offsetX ?? 0;
        this.offsetY = init.offsetY ?? 0;
        this.clientX = init.clientX ?? 0;
        this.clientY = init.clientY ?? 0;
    }
    preventDefault() {}
    stopPropagation() {}
    stopImmediatePropagation() {}
}

const region = new TestHTMLElement();

beforeAll(async () => {
    vi.stubGlobal('HTMLElement', TestHTMLElement);
    vi.stubGlobal('MouseEvent', TestMouseEvent);
    vi.stubGlobal('document', {
        body: { style: { cursor: '' } },
        addEventListener() {},
    });
    Object.assign(window, {
        _wails: { environment: { OS: 'darwin' }, flags: {} },
        webkit: { messageHandlers: { external: { postMessage: (message) => posted.push(message) } } },
        addEventListener: (type, listener) => listeners.set(type, listener),
        getComputedStyle: () => ({ getPropertyValue: (name) => name === '--wails-draggable' ? 'drag' : '' }),
        setInterval: () => 0,
        clearInterval() {},
    });
    await import('./drag');
});

function mouse(type, init) {
    listeners.get(type)(new TestMouseEvent(type, { target: region, offsetX: 10, offsetY: 10, ...init }));
}

beforeEach(() => {
    // End any gesture a previous test left in progress.
    mouse('mouseup', { button: 0, buttons: 0 });
    posted.length = 0;
});

describe('drag', () => {
    it('starts a drag when the primary button is pressed on a draggable region and moved', () => {
        mouse('mousedown', { button: 0, buttons: 1, detail: 1 });
        mouse('mousemove', { buttons: 1 });
        expect(posted).toEqual(['wails:drag']);
    });

    it('does not start a drag when the pointer enters with the primary button already held', () => {
        // The press belongs to another gesture, such as resizing an
        // overlapping window: this window only sees the pointer pass over it.
        mouse('mousemove', { buttons: 1 });
        mouse('mousemove', { buttons: 1 });
        expect(posted).toEqual([]);
    });
});
