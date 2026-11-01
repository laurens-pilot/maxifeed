class ReminderHandler {
    constructor(section) {
        this.section = section;
        this.timeInput = section.querySelector("[data-reminder-time]");
        this.enableButton = section.querySelector("[data-reminder-enable]");
        this.testButton = section.querySelector("[data-reminder-test]");
        this.disableButton = section.querySelector("[data-reminder-disable]");
        this.subscription = null;
    }

    static isSupported() {
        return "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;
    }

    static async currentSubscription() {
        const registration = await navigator.serviceWorker.getRegistration();
        return registration ? registration.pushManager.getSubscription() : null;
    }

    static deviceTimezone() {
        return Intl.DateTimeFormat().resolvedOptions().timeZone;
    }

    static async syncDevice(url) {
        if (!ReminderHandler.isSupported()) return;

        const subscription = await ReminderHandler.currentSubscription();
        if (subscription) {
            sendPOSTRequest(url, { timezone: ReminderHandler.deviceTimezone(), subscription });
        }
    }

    static decodeKey(key) {
        const binary = atob(key.replaceAll("-", "+").replaceAll("_", "/"));
        return Uint8Array.from(binary, (character) => character.charCodeAt(0));
    }

    listen() {
        this.enableButton.addEventListener("click", () => this.run(() => this.enable(), this.section.dataset.labelSaved));
        this.testButton.addEventListener("click", () => this.run(() => this.post(this.section.dataset.testUrl), this.section.dataset.labelTestSent));
        this.disableButton.addEventListener("click", () => this.run(() => this.disable(), this.section.dataset.labelOff));
        this.timeInput.addEventListener("change", () => {
            if (this.subscription && this.timeInput.value) {
                this.run(() => this.save(), this.section.dataset.labelSaved);
            }
        });

        this.render();
        ReminderHandler.syncDevice(this.section.dataset.deviceUrl);
    }

    showPanel(name) {
        this.section.querySelectorAll("[data-reminder-panel]").forEach((panel) => {
            panel.hidden = panel.dataset.reminderPanel !== name;
        });
    }

    async render() {
        if (!ReminderHandler.isSupported()) {
            const canInstall = window.isSecureContext && !InstallHandler.isInstalled();
            this.showPanel(canInstall ? "install" : "unsupported");
            return;
        }

        if (Notification.permission === "denied") {
            this.showPanel("blocked");
            return;
        }

        this.subscription = await ReminderHandler.currentSubscription();
        this.showPanel("form");
        this.enableButton.hidden = this.subscription !== null;
        this.testButton.hidden = this.subscription === null;
        this.disableButton.hidden = this.subscription === null;
    }

    async run(action, successLabel) {
        this.section.querySelectorAll("button").forEach((button) => {
            button.disabled = true;
        });

        try {
            if (await action() !== false) {
                showToastNotification("read", successLabel);
            }
        } catch (error) {
            showToastNotification("unread", error.message || this.section.dataset.labelError);
        }

        this.section.querySelectorAll("button").forEach((button) => {
            button.disabled = false;
        });
        await this.render();
    }

    async post(url, body = null) {
        const response = await sendPOSTRequest(url, body);
        if (!response.ok) {
            const details = await response.json().catch(() => ({}));
            throw new Error(details.error_message);
        }
    }

    async enable() {
        const permission = await Notification.requestPermission();
        if (permission !== "granted") return false;

        const registration = await navigator.serviceWorker.ready;
        this.subscription = await registration.pushManager.subscribe({
            userVisibleOnly: true,
            applicationServerKey: ReminderHandler.decodeKey(this.section.dataset.publicKey),
        });

        await this.save();
    }

    save() {
        return this.post(this.section.dataset.saveUrl, {
            time: this.timeInput.value,
            timezone: ReminderHandler.deviceTimezone(),
            subscription: this.subscription,
        });
    }

    async disable() {
        const { endpoint } = this.subscription;
        await this.subscription.unsubscribe();
        await this.post(this.section.dataset.removeUrl, { endpoint });
    }
}
