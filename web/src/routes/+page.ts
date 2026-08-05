import { redirect } from "@sveltejs/kit";

// The dashboard has no home page of its own: the setup flow is the entry
// point (token gate). Advertised URL is "/", so root redirects there.
export const load = () => {
  throw redirect(307, "/setup");
};
