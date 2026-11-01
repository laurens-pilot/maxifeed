const INSTALL_CARD_DISMISSED_STORAGE_KEY = "install-card-dismissed";

class InstallHandler {
    constructor() {
        this.sheet = document.getElementById("install-sheet");
        this.card = document.querySelector(".install-card");
        this.deferredPrompt = null;
    }

    static isInstalled() {
        return navigator.standalone === true || window.matchMedia("(display-mode: standalone)").matches;
    }

    static isCardDismissed() {
        try {
            return localStorage.getItem(INSTALL_CARD_DISMISSED_STORAGE_KEY) !== null;
        } catch {
            return true;
        }
    }

    listen() {
        if (InstallHandler.isInstalled()) return;

        window.addEventListener("beforeinstallprompt", (event) => {
            event.preventDefault();
            this.deferredPrompt = event;
        });
        window.addEventListener("appinstalled", () => this.hideOffers());

        document.querySelectorAll("[data-install]").forEach((button) => {
            button.hidden = false;
            button.addEventListener("click", () => this.install());
        });

        if (this.card && window.matchMedia("(pointer: coarse)").matches && !InstallHandler.isCardDismissed()) {
            this.card.hidden = false;
            this.card.querySelector("[data-install-dismiss]").addEventListener("click", () => this.dismissCard());
        }
    }

    hideOffers() {
        document.querySelectorAll("[data-install]").forEach((button) => {
            button.hidden = true;
        });

        if (this.card) {
            this.card.hidden = true;
        }
    }

    dismissCard() {
        this.card.hidden = true;

        try {
            localStorage.setItem(INSTALL_CARD_DISMISSED_STORAGE_KEY, "true");
        } catch {
            return;
        }
    }

    async install() {
        if (!this.deferredPrompt) {
            this.sheet.showPopover();
            return;
        }

        document.getElementById("more-sheet").hidePopover();
        this.deferredPrompt.prompt();
        await this.deferredPrompt.userChoice;
        this.deferredPrompt = null;
    }
}
