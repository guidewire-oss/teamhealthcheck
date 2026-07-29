# Team Health Check Frontend

Next.js 15 application for the Team Health Check Squad Health Check system.

## Quick Start

From the root of the monorepo:
```bash
make run-frontend
```

Or from this directory:
```bash
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000)

## Mac ARM64 Troubleshooting

If you encounter SWC-related errors on Mac ARM64:

```bash
# Clear cache and reinstall
npm cache clean --force
rm -rf node_modules package-lock.json .next
npm install

# If still having issues, force reinstall SWC
npm install --force @next/swc-darwin-arm64
```

## Login Credentials

All passwords are "demo" except admin ("admin"):

- **Vice President**: `vp/demo`
- **Directors**: `director1/demo`, `director2/demo`
- **Managers**: `manager1/demo`, `manager2/demo`, `manager3/demo`
- **Team Leads**: `teamlead1/demo` through `teamlead9/demo`
- **Team Members**: `demo/demo`, `alice/demo`, etc.
- **Administrator**: `admin/admin`

## Technology Stack

- **Framework**: Next.js 15 with App Router
- **Language**: TypeScript (strict mode)
- **Styling**: Tailwind CSS
- **Charts**: Recharts
- **Icons**: Lucide React
- **Forms**: React Hook Form
- **State Management**: Client-side with localStorage (will migrate to API)
- **Authentication**: Cookie-based with js-cookie (will migrate to JWT)

## Project Structure

```
frontend/
├── app/                  # Next.js App Router
│   ├── login/           # Authentication page
│   ├── survey/          # Health check survey
│   ├── dashboard/       # Team Lead dashboard
│   ├── manager/         # Manager/Director/VP dashboard
│   ├── admin/           # Administration panel
│   └── layout.tsx       # Root layout
├── lib/                 # Utilities and business logic
│   ├── auth.ts         # Authentication logic
│   ├── data.ts         # Data management (localStorage)
│   ├── types.ts        # TypeScript type definitions
│   ├── org-config.ts   # Organization hierarchy
│   ├── teams-data.ts   # Team data and mock generator
│   └── assessment-period.ts  # Automatic period detection
├── components/          # React components
├── middleware.ts        # Route protection
├── public/             # Static assets
└── package.json        # Dependencies
```

## Key Features

### Survey Experience
- Step-by-step wizard (11 health dimensions)
- Automatic assessment period detection based on submission date
- Visual progress tracking
- Intuitive red/yellow/green selection
- Trend indicators (improving/stable/declining)
- Optional comments for context

### Manager/Team Lead Dashboard
- **Overview Tab**: Radar chart and distribution graphs
- **Details Tab**: Dimension-by-dimension breakdown
- **Trends Tab**: Historical data with period filtering
- Team statistics and next check reminders
- Hierarchical access control (see only relevant teams)

### Admin Panel
- Team management (CRUD)
- User administration
- Cadence configuration
- Hierarchy level management
- System settings

## Data Architecture

Currently uses **localStorage for persistence** (demo mode):
- Health check sessions: `healthCheckSessions`
- Organization config: `orgConfig`
- User authentication: Cookies (1-day expiration)

**Migration Plan**: Will be replaced with API calls to the Go backend.

## Development

### Available Scripts

```bash
npm run dev     # Start development server
npm run build   # Production build
npm start       # Start production server
```

### Path Aliases

TypeScript paths are configured with `@/*` alias:
```typescript
import { User } from '@/lib/types';
import { getCurrentUser } from '@/lib/auth';
```

## Architecture Notes

See [CLAUDE.md](../CLAUDE.md) for comprehensive architecture documentation including:
- Domain-Driven Design concepts
- Hierarchy and access control system
- Supervisor chain patterns
- Assessment period logic
- State management patterns

## Future Enhancements

- [ ] Migrate from localStorage to API calls
- [ ] Replace cookie auth with JWT tokens
- [ ] Add frontend unit tests (Jest/Vitest)
- [ ] Add E2E tests (Playwright/Cypress)
- [ ] Improve mobile responsiveness
- [ ] Add internationalization (i18n)
- [ ] Implement real-time updates
- [ ] Add data export functionality

## Contributing

1. Follow TypeScript strict mode
2. Use Tailwind CSS for styling (no inline styles)
3. Maintain component reusability
4. Add JSDoc comments for complex functions
5. Test on both Mac ARM64 and x86 platforms

## Learn More

- [Root README](../README.md) - Monorepo overview
- [Backend README](../backend/README.md) - Go API details
- [CLAUDE.md](../CLAUDE.md) - Development guide
- [Next.js Documentation](https://nextjs.org/docs)
