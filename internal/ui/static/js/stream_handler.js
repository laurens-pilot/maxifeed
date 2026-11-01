const STREAM_SWIPE_SLOP = 12;
const STREAM_SWIPE_THRESHOLD = 96;
const STREAM_READ_FLUSH_DELAY = 1500;
const STREAM_FEED_CHECK_DELAY = 4000;
const STREAM_HINT_STORAGE_KEY = "stream-hint-seen";
const READER_DISMISS_DISTANCE = 120;
const CONFETTI_PIECES = 36;

class StreamHandler {
    constructor(stream) {
        this.stream = stream;
        this.autoRead = stream.dataset.autoRead === "true";
        this.reader = document.getElementById("reader");
        this.readerBody = this.reader.querySelector(".reader-body");
        this.readerSave = this.reader.querySelector("[data-reader-save]");
        this.readerOriginal = this.reader.querySelector("[data-reader-original]");
        this.progressFill = document.querySelector(".stream-progress-fill");
        this.progressCount = document.querySelector(".stream-count-done");
        this.totalCards = stream.querySelectorAll(".card").length;
        this.htmlPolicy = trustedTypes.createPolicy("html", { createHTML: (html) => html });
        this.reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
        this.doneCards = new Set();
        this.pendingReads = new Set();
        this.flushTimer = null;
        this.openCard = null;
        this.swipe = null;
        this.readerDrag = null;
        this.ignoreNextClick = false;
    }

    listen() {
        this.stream.addEventListener("click", (event) => this.onClick(event));
        this.stream.addEventListener("error", (event) => this.onImageError(event), true);
        this.stream.addEventListener("pointerdown", (event) => this.onPointerDown(event));
        this.stream.addEventListener("pointermove", (event) => this.onPointerMove(event));
        this.stream.addEventListener("pointerup", () => this.onPointerUp());
        this.stream.addEventListener("pointercancel", () => this.cancelSwipe());

        this.reader.addEventListener("click", (event) => this.onReaderClick(event));
        this.reader.addEventListener("cancel", (event) => {
            event.preventDefault();
            history.back();
        });
        this.reader.addEventListener("touchstart", (event) => this.onReaderTouchStart(event), { passive: true });
        this.reader.addEventListener("touchmove", (event) => this.onReaderTouchMove(event), { passive: false });
        this.reader.addEventListener("touchend", () => this.onReaderTouchEnd());

        window.addEventListener("popstate", () => this.closeReader());
        window.addEventListener("pagehide", () => this.flushReads(true));
        document.addEventListener("visibilitychange", () => {
            if (document.visibilityState === "hidden") {
                this.flushReads(true);
            }
        });

        if (this.autoRead) {
            history.scrollRestoration = "manual";
            this.observeCards();
            this.observeFinishLine();
            this.showHint();
        }

        document.querySelector("[data-stream-next]")?.addEventListener("click", (event) => {
            event.preventDefault();
            this.flushReads().finally(() => window.location.assign(event.target.href));
        });
    }

    observeCards() {
        const headerHeight = document.querySelector(".stream-header")?.offsetHeight ?? 0;
        const observer = new IntersectionObserver((records) => {
            records.forEach(({ isIntersecting, boundingClientRect, rootBounds, target }) => {
                if (!isIntersecting && boundingClientRect.bottom <= rootBounds.top) {
                    this.onCardPassed(target);
                }
            });
        }, { rootMargin: `-${headerHeight}px 0px 0px 0px` });

        this.stream.querySelectorAll(".card").forEach((card) => observer.observe(card));
    }

    observeFinishLine() {
        const finishLine = document.querySelector(".finish");
        const observer = new IntersectionObserver((records) => {
            if (records.some((record) => record.isIntersecting)) {
                observer.disconnect();
                this.celebrate(finishLine);
            }
        }, { threshold: 0.4 });

        observer.observe(finishLine);
    }

    showHint() {
        try {
            if (localStorage.getItem(STREAM_HINT_STORAGE_KEY)) return;
            localStorage.setItem(STREAM_HINT_STORAGE_KEY, "true");
        } catch {
            return;
        }

        this.stream.querySelector(".stream-hint").hidden = false;
    }

    celebrate(finishLine) {
        if (this.reducedMotion) return;

        for (let index = 0; index < CONFETTI_PIECES; index++) {
            const piece = document.createElement("i");
            piece.className = "confetti";
            piece.style.setProperty("--confetti-x", `${Math.random() * 100}%`);
            piece.style.setProperty("--confetti-drift", `${(Math.random() - 0.5) * 120}px`);
            piece.style.setProperty("--confetti-hue", `${Math.floor(Math.random() * 360)}`);
            piece.style.setProperty("--confetti-delay", `${Math.random() * 0.4}s`);
            piece.addEventListener("animationend", () => piece.remove());
            finishLine.appendChild(piece);
        }
    }

    onCardPassed(card) {
        card.dataset.passed = "true";
        this.markDone(card);

        if (card.dataset.starred !== "true") {
            this.markRead(card);
        }
    }

    markDone(card) {
        this.doneCards.add(card.dataset.id);

        if (this.progressFill) {
            this.progressFill.style.transform = `scaleX(${this.doneCards.size / this.totalCards})`;
            this.progressCount.textContent = this.doneCards.size;
        }
    }

    markRead(card) {
        if (card.dataset.status === "read") return;

        card.dataset.status = "read";
        this.pendingReads.add(Number(card.dataset.id));

        clearTimeout(this.flushTimer);
        this.flushTimer = setTimeout(() => this.flushReads(), STREAM_READ_FLUSH_DELAY);
    }

    async flushReads(keepalive = false) {
        clearTimeout(this.flushTimer);
        if (this.pendingReads.size === 0) return;

        const entryIDs = [...this.pendingReads];
        this.pendingReads.clear();

        try {
            const response = await sendPOSTRequest(this.stream.dataset.readUrl, { entry_ids: entryIDs }, keepalive);
            if (!response.ok) {
                throw new Error(`unexpected status ${response.status}`);
            }
            const count = await response.json();
            updateUnreadCounterValue(-count);
            this.creditReads(count);
        } catch {
            entryIDs.forEach((entryID) => this.pendingReads.add(entryID));
        }
    }

    creditReads(count) {
        document.querySelectorAll(".read-today-count").forEach((element) => {
            element.textContent = Number(element.textContent) + count;
        });

        const streak = document.querySelector(".streak");
        if (count > 0 && streak) {
            streak.hidden = false;
            document.querySelectorAll(".streak-count").forEach((element) => {
                element.textContent = streak.dataset.nextStreak;
            });
        }
    }

    setSaved(card, saved) {
        card.dataset.starred = saved;
        card.querySelector("[data-card-save]").setAttribute("aria-pressed", saved);

        if (card === this.openCard) {
            this.readerSave.setAttribute("aria-pressed", saved);
        }
    }

    async toggleSave(card) {
        const saved = card.dataset.starred !== "true";
        this.setSaved(card, saved);
        navigator.vibrate?.(10);

        const response = await sendPOSTRequest(card.dataset.starUrl);
        if (!response.ok) {
            this.setSaved(card, !saved);
            return;
        }

        if (saved && this.autoRead) {
            this.keepUnread(card);
        } else if (card.dataset.passed === "true") {
            this.markRead(card);
        }
    }

    keepUnread(card) {
        if (card.dataset.status !== "read") return;

        card.dataset.status = "unread";
        if (!this.pendingReads.delete(Number(card.dataset.id))) {
            updateEntriesStatus([Number(card.dataset.id)], "unread");
        }
    }

    skip(card) {
        if (!this.autoRead && card.dataset.starred === "true") {
            sendPOSTRequest(card.dataset.starUrl);
        }

        this.markDone(card);
        this.markRead(card);
        this.removeCard(card);
    }

    async removeCard(card) {
        if (!this.reducedMotion) {
            const options = { duration: 180, easing: "ease-in", fill: "forwards" };
            await card.animate([{ transform: "translateX(-110%)", opacity: 0 }], options).finished;
            await card.animate([{ height: 0, marginBottom: "-14px" }], options).finished;
        }
        card.remove();
    }

    async share(card) {
        const { title, link } = card.dataset;

        if (navigator.share) {
            await navigator.share({ title, url: link }).catch(() => {});
        } else if (navigator.clipboard) {
            await navigator.clipboard.writeText(link);
            showToastNotification("read", this.stream.dataset.labelCopied);
        }
    }

    onClick(event) {
        if (this.ignoreNextClick) {
            this.ignoreNextClick = false;
            event.preventDefault();
            return;
        }

        const card = event.target.closest(".card");
        if (!card) return;

        const control = event.target.closest("a, button");
        if (control?.matches("[data-card-save]")) {
            this.toggleSave(card);
        } else if (control?.matches("[data-card-share]")) {
            this.share(card);
        } else if (control?.matches("[data-card-skip]")) {
            this.skip(card);
        } else if (!control || control.matches("[data-open-reader]")) {
            if (event.ctrlKey || event.metaKey || event.shiftKey) return;
            event.preventDefault();
            this.openReader(card);
        }
    }

    onImageError(event) {
        if (event.target.matches(".card-image")) {
            event.target.remove();
        }
    }

    onPointerDown(event) {
        this.ignoreNextClick = false;
        if (event.pointerType !== "touch" || !event.isPrimary) return;

        const card = event.target.closest(".card");
        if (card) {
            this.swipe = {
                card,
                face: card.querySelector(".card-face"),
                startX: event.clientX,
                distance: 0,
                active: false,
                armed: false,
            };
        }
    }

    onPointerMove(event) {
        if (!this.swipe) return;

        const { card, face, startX } = this.swipe;
        const distance = event.clientX - startX;

        if (!this.swipe.active) {
            if (Math.abs(distance) < STREAM_SWIPE_SLOP) return;
            this.swipe.active = true;
            card.classList.add("card-swiping");
        }

        const armed = Math.abs(distance) >= STREAM_SWIPE_THRESHOLD;
        if (armed !== this.swipe.armed) {
            navigator.vibrate?.(8);
        }

        this.swipe.distance = distance;
        this.swipe.armed = armed;
        card.dataset.swipe = distance > 0 ? "save" : "skip";
        card.classList.toggle("card-armed", armed);
        face.style.transform = `translateX(${distance}px) rotate(${distance / 70}deg)`;
    }

    onPointerUp() {
        if (!this.swipe) return;

        const { card, distance, active, armed } = this.swipe;
        this.cancelSwipe();
        this.ignoreNextClick = active;

        if (armed && distance > 0) {
            this.toggleSave(card);
        } else if (armed) {
            this.skip(card);
        }
    }

    cancelSwipe() {
        if (!this.swipe) return;

        const { card, face } = this.swipe;
        this.swipe = null;
        card.classList.remove("card-swiping", "card-armed");
        delete card.dataset.swipe;
        face.style.transform = "";
    }

    async openReader(card) {
        this.openCard = card;
        this.readerSave.setAttribute("aria-pressed", card.dataset.starred);
        this.readerOriginal.href = card.dataset.link;
        this.reader.showModal();
        this.reader.scrollTop = 0;
        history.pushState({ reader: true }, "");

        if (!this.reducedMotion) {
            this.reader.animate([{ transform: "translateY(100%)" }, { transform: "translateY(0)" }], { duration: 280, easing: "cubic-bezier(0.2, 0.9, 0.3, 1)" });
        }

        const response = await fetch(card.dataset.readerUrl);
        if (!response.ok) {
            window.location.assign(card.querySelector("[data-open-reader]").href);
            return;
        }

        const html = await response.text();
        if (this.openCard === card) {
            this.readerBody.innerHTML = this.htmlPolicy.createHTML(html);
        }
    }

    async closeReader() {
        if (!this.openCard) return;

        const card = this.openCard;
        this.openCard = null;

        if (!this.reducedMotion) {
            const options = { duration: 200, easing: "ease-in", fill: "forwards" };
            await this.reader.animate([{ transform: "translateY(100%)" }], options).finished;
        }

        this.reader.close();
        this.reader.getAnimations().forEach((animation) => animation.cancel());
        this.reader.style.transform = "";
        this.readerBody.textContent = "";

        this.markDone(card);
        if (!this.autoRead || card.dataset.starred !== "true") {
            this.markRead(card);
        }
    }

    onReaderClick(event) {
        const control = event.target.closest("a, button");

        if (control?.matches("[data-reader-close]")) {
            history.back();
        } else if (control?.matches("[data-reader-save]")) {
            this.toggleSave(this.openCard);
        } else if (control?.matches("[data-reader-full]")) {
            this.loadFullArticle(control);
        } else if (control?.matches("[data-reader-send]")) {
            this.sendToIntegrations(control);
        }
    }

    async sendToIntegrations(button) {
        button.disabled = true;

        const response = await sendPOSTRequest(button.dataset.url);
        if (response.ok) {
            button.textContent = button.dataset.labelDone;
        } else {
            button.disabled = false;
        }
    }

    async loadFullArticle(button) {
        button.disabled = true;

        const response = await sendPOSTRequest(button.dataset.url);
        if (!response.ok) {
            button.disabled = false;
            return;
        }

        const { content } = await response.json();
        if (content) {
            this.readerBody.querySelector(".entry-content").innerHTML = this.htmlPolicy.createHTML(content);
        }
        button.remove();
    }

    onReaderTouchStart(event) {
        this.readerDrag = this.reader.scrollTop <= 0 ? { startY: event.touches[0].clientY, distance: 0 } : null;
    }

    onReaderTouchMove(event) {
        if (!this.readerDrag) return;

        const distance = event.touches[0].clientY - this.readerDrag.startY;
        if (distance <= 0) {
            this.readerDrag = null;
            this.reader.style.transform = "";
            return;
        }

        event.preventDefault();
        this.readerDrag.distance = distance;
        this.reader.style.transform = `translateY(${distance}px)`;
    }

    onReaderTouchEnd() {
        if (!this.readerDrag) return;

        const { distance } = this.readerDrag;
        this.readerDrag = null;

        if (distance >= READER_DISMISS_DISTANCE) {
            history.back();
        } else {
            this.reader.style.transform = "";
        }
    }

    static checkFeeds(button) {
        button.disabled = true;
        showToastNotification("read", button.dataset.labelChecking);

        sendPOSTRequest(document.body.dataset.refreshAllFeedsUrl).finally(() => {
            setTimeout(() => window.location.reload(), STREAM_FEED_CHECK_DELAY);
        });
    }
}
