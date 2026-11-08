import type { Config } from "@react-router/dev/config";

// Server-rendered (plan D8): product, live and recap links shared on KakaoTalk
// need a title, price and photo in the HTML.
export default { ssr: true } satisfies Config;
