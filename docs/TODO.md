## TODO

## Infra

- [x] Firewalls
- [x] Self hosted grafana for logs and metrics
- [x] Cloudflare DNS
- [x] Ingress contract docs: MCP on `https://mcp.ml.ink/mcp`, webhooks on `https://api.ml.ink`, Cloudflare LB as source of truth

## Backend

- Provision SQLite
- Provision Postgres
- [x] MCP tool resources

## Product

- UI for all MCP clients
- Consider SKILL.md with API

## k3s

- For namespace let's use `userid-projectref`, not `github-projectref`. What if users change their github/gitea.
- Make `userid` short UUID
- Track-only: enforce pod-level `runAsNonRoot` + `allowPrivilegeEscalation=false` for tenant template (`infra/k8s/customer-service-template.yml`) after compatibility validation.
- If user deletes service while it's being deployed the deployment service will stop and likely will not clean itself
- make sure all ansible manifests match the infrastructure, nothing should be provisioned manually
- let's not show graphql errors like `errors="input: me failed to get Firebase user: context canceled\n"` it means user refreshed the page before query loaded.
- Separate binaries for `mcp` and `graphql` API.
- Should we have sepparate application.yaml for mcp, deplyer, server/worker?
- If agent deploys app on wrong port there should be some mechanism for it to report it as error. Currently it does WaitForRollout until it timesout many times which is weird.
- Prepare terminal for making videos.
- App store/gallery.
- Allow deploying custom domains.
- Substack mlink
- Free tier on a different runner node
- How to make it testable? (test cluster on few smaller nodes?)
- How to position? Exactly what's my offering?
- Terms of service, payments, discount codes.
- Is it possible to implement support agent that has read only access to single account via k8s service account?
- How to make nice visual content that is branded as Ink?
- What is Ink logo?
- Port conclifc error message within a namespace.
- One design decision is that maybe agent doesn't need gitea, we can just send tarbal with some command like curl or wget
- how to get internal IP names, does it work already within namespace (service-name.internal)?
- What's the right way to handle preview URLs?
- Create stack inside a project (nextjs-postgres)?
- Can I allow people to run things like postgres + mounted volume? Same as railway, no guarantees.
- Regenerate FEATURE-PLAN based on current codebase.
- feedback - tool
- docs search tool
- when volume is mounted, how agent puts data in there? Should I allow ssh into instance?
- metrics via tool
- propose good schema for list_services and get_service/service_details
- estimate cost tool
- figure out cost
  - how much 1GB of disk
  - how much 1 vCPU
  - how much 1 GB RAM
- how to have docs in seprate public repo, but smoothly integrate in ml.ink/docs? Worse case scenario is docs.ml.ink Build docs from scratch or use off the shelf? I want docs to be fully compatible with agents with whatever it means (.agents), robots allowed,
- do analysis on all other platforms with MCP (vercel, railway, ...). What they offer that I dont? Are mcp tools nice? What is unique about me?
- what provider to use for US region?
- disk metrics on Grafana. What happens when too many versions of docker images? How many images can I store of my users? Is auto deletion of older versions handled - I should store 2 images, current and last? Where images stored - builder or ops?
- Restructure /infra folder. Directory structure should make sense assuming we will have multiple regions ultimately, each region it's own k8s/k3s cluster.

ok i can build these yaml DSLs for generating videos, but what engine runs these? how to handle nice smooth zoom that scren studio does locateOnScreen image that pyaotogui has is super powerful, it can work well for me. Explain how would I implement all primitives: is the stack here becomes OBS + pyaotogui? One thing screen studio does well is that videos look cool. Around a frame it adds nice background making it seem like you're recording the app. Given the video ideas you have in the list create list of primitives, explain what concept videos can be automate which cannot. For example on top I can add AI avatar moving. Based on DSL/yaml agent can generate transcript and speach for 11labs and then furtuermore talking avatar

For every video idea that we discussed I want you to add relevant tools or we hack our own tools

This is cool, can I define some primitives describing reusable operations that can be composed later on. For example "add_mcp_server_claude_code" this executes sequence of operations and can by itself be embedded. What do you propose yaml, json or just pure python? Ultimately I want LLM to write these DSL/code. Propose DSL, give some examples. This DSL should span to both capture and composition to produce final video. Avatar and voice is optional of course. For now we can keep them as separte steps/scripts to iterate independently fast, but ultimately single DSL/script should capture full video generation. Propose best products for generating avatars from voice (heygen is good, but we can also use . In some cases like tiktok i imagine we will make videos such as video is top and bottom has avatar speaking. Propose list of composable primitives. Maybe it's better if AI writes python files where these DSL operations are typed? Or maybe it doesn't matter as we define schema and options. For example actions incorporate both capture and post production actions? For example mark_speedup {factor: 4}. It is really weird schema, because it looks like yaml and json at the same time, yaml just have nested things no brackets
