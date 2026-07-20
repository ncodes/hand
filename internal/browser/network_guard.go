package browser

const browserNetworkGuardScript = `(() => {
  const blocked = (name) => {
    Object.defineProperty(globalThis, name, {
      configurable: false,
      value: class {
        constructor() {
          throw new DOMException(name + " is disabled", "SecurityError");
        }
      },
      writable: false
    });
  };
  blocked("WebSocket");
  blocked("Worker");
  blocked("SharedWorker");
  if (navigator.serviceWorker) {
    Object.defineProperty(navigator.serviceWorker, "register", {
      configurable: false,
      value: () => Promise.reject(new DOMException("Service workers are disabled", "SecurityError")),
      writable: false
    });
  }
})();`
