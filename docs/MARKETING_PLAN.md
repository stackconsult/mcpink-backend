Here's a broad marketing strategy. Some of these are conventional, some are weird. The weird ones are the ones that go viral.

## Video Content

**The obvious stack demos (do these first, they're SEO workhorses):**

- "Claude deploys a full-stack Next.js app in 47 seconds"
- "Cursor builds and deploys a Flask API with SQLite — zero config"
- "Agent builds a SaaS from a napkin sketch" (give it a screenshot of a hand-drawn wireframe)
- "Deploying a Go backend + React frontend with one conversation"
- Every popular framework gets its own video. These are long-tail search magnets.

**The self-referential ones (these get shared because they're meta):**

- "I asked Claude to deploy itself" — clonebot idea, great hook
- "AI agent deploys an MCP server that deploys other MCP servers" — recursion is inherently shareable
- "I gave an agent my startup idea and went to lunch. It was live when I got back." — timelapse, real clock on screen
- "Claude fixes its own production bug" — agent sees crash in logs, reads the error, pushes a fix, redeploys. The full loop with no human intervention.

**The competition bait (controversial = engagement):**

- "Deploying to ml.ink vs Railway vs Vercel — which one can an agent actually use?" — side by side, same prompt, three platforms. You win because yours is purpose-built for this.
- "How many tool calls does it take to deploy on Railway MCP?" — count them. Then show yours. The gap is the marketing.
- "I tried to deploy a backend on Vercel" — short, painful, ends with switching to ml.ink. Relatable frustration content.

**The absurd ones (these go viral on Twitter/TikTok):**

- "Agent deploys 100 apps in 10 minutes" — speedrun. Show the counter going up. Absurd scale is inherently shareable.
- "I let an AI run my infrastructure for a week" — daily diary format. What did it deploy? Did anything break? How did it fix things?
- "Non-technical person deploys a full-stack app by just describing it" — get your mom/friend/whoever to talk to Claude. The less technical the better. "Make me a website where people can rate their neighbor's cats." Boom, deployed.
- "Agent vs junior developer: who deploys faster?" — race format. The agent wins. The junior dev is a good sport about it. People share this because it's funny and slightly threatening.
- "I asked 5 different AI models to deploy an app. Only one succeeded." — model comparison content gets massive engagement in the AI community.

**The educational ones (build trust, establish authority):**

- "How MCP actually works — explained by building a deploy platform" — technical deep dive, positions you as the expert
- "Why every AI agent will need a deploy tool" — thought leadership, vision piece
- "The architecture behind ml.ink" — developers love behind-the-scenes infra content. Show k3s, Longhorn, Temporal, the whole stack.

## Written Content

**Blog posts that earn backlinks:**

- "The Agent Deploy Problem: Why AI needs infrastructure designed for it" — the manifesto. This is your founding narrative. Why you built this, why existing tools fail agents, what agent-first means. Post on your blog, crosspost to HN.
- "We analyzed 10,000 agent deployments. Here's what they build." — once you have usage data, this is gold. What frameworks are popular? What errors are common? What do agents struggle with? Original data always gets shared.
- "MCP Tool Design: What we learned building for AI agents" — practical lessons about error messages, tool counts, response design. Developers will bookmark this. It's useful even if they never use your product.
- "How to make your app deployable by an agent in 5 minutes" — practical guide. Add a Dockerfile, expose the right port, use env vars for config. Positions ml.ink as the obvious destination.

**SEO content (boring but effective):**

- "How to deploy [framework] with AI" for every framework: Next.js, Flask, Django, Express, FastAPI, Go, Rails, Laravel, Rust Axum, etc. Each one ranks for "[framework] deploy" searches.
- "MCP server for deployment" — own this search term before anyone else does
- "Claude deploy app", "Cursor deploy backend", "AI deploy full stack" — every search term an early adopter might type

## Getting ml.ink Tokens Into The World

**MCP directory listings:**

- Get listed on every MCP directory, registry, and awesome-list. Smithery, Glama, awesome-mcp-servers on GitHub. These are where agents and developers discover MCP tools today.
- Write the best README in every directory. The README is your storefront.

**Cursor/Windsurf/Claude integrations:**

- Write dedicated setup guides for each client: "Add ml.ink to Cursor in 30 seconds", "One-click ml.ink for Claude Desktop". Make it trivially easy.
- If any of these clients have featured/recommended MCP servers, get on that list. Reach out to their DevRel.

**Open source presence:**

- The MCP server itself should be open source. Developers trust what they can read. It also means every GitHub search for "deploy MCP" finds you.
- Contribute to MCP ecosystem projects. PRs, issues, discussions. Be visible in the community.
- Create example repos: `ml-ink-nextjs-template`, `ml-ink-flask-starter`, etc. Each one is a backlink and a discovery surface.

**Developer community seeding:**

- Post every video and blog post to Hacker News, r/programming, r/artificial, r/ChatGPT, r/cursor, relevant Discord servers
- Don't spam — contribute. Answer questions about MCP, deployment, agent tooling. Become the person people associate with "agent deployment."
- Find threads where people complain about deploying from AI agents. "I built my app in Cursor but then spent 2 hours figuring out how to deploy it." That's your opening.

**The referral/word-of-mouth mechanics:**

- Every deployed app at `*.ml.ink` is a walking billboard. The subdomain IS the marketing. When someone shares their app URL, they're sharing your brand.
- "Deployed with ml.ink" badge/footer — opt-in, maybe give extra free tier for displaying it. Like Vercel's "▲ Powered by Vercel" but for agent deploys.
- Free tier must be generous enough that people actually use it and share URLs. The free tier is marketing spend, not lost revenue.

**Partnerships/integrations:**

- Reach out to AI coding tool creators (Cursor, Windsurf, Cline, Aider, Claude Code). Offer to be their recommended deploy target. They all have the same problem: users build apps but can't deploy them easily.
- Reach out to AI tutorial creators on YouTube. Offer them free accounts. Every tutorial that ends with "...and deploy to ml.ink" is an ad you didn't pay for.
- Turso partnership — you're already using them. Cross-promote. "The best SQLite deploy stack: Turso + ml.ink."

**The unconventional plays:**

- **Agent-built landing pages:** Let people type a description and get a live landing page at `something.ml.ink` in 30 seconds. Free. Viral because people will share their generated pages. Each one has your domain in the URL.
- **Deploy challenge:** "Deploy something cool → post the ml.ink URL → best one wins [prize]." Community contest. Generates content, showcases the platform, creates buzz.
- **MCP benchmark/leaderboard:** Create a public benchmark that tests how well different platforms work with AI agents. Number of tool calls, time to deploy, error recovery success rate. You'll win your own benchmark (because you designed for it), but making it open and reproducible builds credibility.
- **"Deploy it for me" bot on Twitter/Reddit:** People post code screenshots or repo links, tag your bot, it deploys it and replies with the URL. Extremely visible, extremely shareable. Every reply is an ad.
- **Agent hackathon sponsorship:** Sponsor AI hackathons. Offer ml.ink as the deployment layer. Every team that uses you during a hackathon becomes a potential long-term user, and hackathon projects get shared widely.

## Priority Order

**Week 1-2:** MCP directory listings, open source the server, Cursor/Claude setup guides. This is distribution infrastructure — everything else builds on it.

**Week 3-4:** First 5 stack demo videos (Next.js, Flask, Express, Go, React+API). Post everywhere. These are your proof points.

**Week 5-6:** The manifesto blog post ("The Agent Deploy Problem"). The self-referential video ("Claude deploys itself"). The competition video. These are your shareable/viral pieces.

**Ongoing:** One video per week, one blog post every two weeks, constant community presence. Consistency beats virality.

The core insight for all of this: **every ml.ink URL in the wild is marketing.** The product markets itself every time someone shares what they built. Your job is to make it so easy that people deploy things they wouldn't have bothered deploying otherwise — and then share the URL because it just works.
