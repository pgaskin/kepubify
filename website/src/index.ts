/* eslint-disable @typescript-eslint/no-non-null-assertion */
import * as Sentry from "@sentry/browser"
import { KepubifyApp } from "./app"

Sentry.init({
    dsn: "https://4ae9999809ef418b819b16bf383e5fd0@o143001.ingest.sentry.io/5828167",
    maxBreadcrumbs: 100,
    attachStacktrace: true,
})

const app = new KepubifyApp("#try")

app.start()
    .catch(ex => Sentry.captureException(new Error(`failed to start kepubify: ${ex}`)))
document.getElementById("unsupported-browser")!.style.display = "none"

// note: in-app browsers often can't download files (the app has to handle it)
if (/\b(GSA|FBAN|FBAV|FB_IAB|Line|MicroMessenger)\/|\b(Instagram|LinkedInApp|Snapchat|musical_ly|BytedanceWebview)\b|; wv\)/.test(navigator.userAgent)) {
    document.getElementById("in-app-browser")!.classList.remove("hidden")
}

export default app
