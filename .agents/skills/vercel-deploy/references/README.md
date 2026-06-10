# Vercel Deployment - Reference Documentation

## Supported Frameworks

Vercel automatically detects and configures these frameworks:

### JavaScript/TypeScript Frameworks
- **Next.js** (App Router & Pages Router)
- **React** (Create React App, Vite)
- **Vue.js** (Vue, Nuxt)
- **Svelte** (SvelteKit)
- **Angular**
- **Astro**
- **Remix**
- **SolidStart**

### Static Sites
- **HTML/CSS/JS** (static files)
- **Jekyll**, **Hugo**, **Gatsby**
- **Docusaurus**, **Nextra**

### Edge Functions
- **Next.js Middleware**
- **Vercel Edge Functions**
- **Cloudflare Workers API**

## Deployment Configuration

### vercel.json
```json
{
  "builds": [
    {
      "src": "package.json",
      "use": "@vercel/next"
    }
  ],
  "routes": [],
  "env": {}
}
```

### Environment Variables
- Set in Vercel dashboard or via CLI
- Can be linked to .env files
- Different values for preview/production

## Deployment Methods

### 1. Vercel CLI
```bash
npm i -g vercel
vercel --prod
```

### 2. GitHub Integration
- Connect repository
- Automatic deployments on push
- Preview deployments for PRs

### 3. Vercel API
- Programmatic deployments
- CI/CD integration

## Resources
- [Vercel Documentation](https://vercel.com/docs)
- [Framework Presets](https://vercel.com/docs/frameworks)
- [CLI Reference](https://vercel.com/docs/cli)
