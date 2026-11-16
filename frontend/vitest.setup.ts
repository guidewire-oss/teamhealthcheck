import '@testing-library/jest-dom'

global.ResizeObserver = class ResizeObserver {
  callback: ResizeObserverCallback
  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
  }
  observe(target: Element) {
    this.callback(
      [{ target, contentRect: { width: 500, height: 500 } } as ResizeObserverEntry],
      this
    )
  }
  unobserve() {}
  disconnect() {}
}

Object.defineProperty(HTMLElement.prototype, 'offsetWidth', { configurable: true, value: 500 })
Object.defineProperty(HTMLElement.prototype, 'offsetHeight', { configurable: true, value: 500 })
