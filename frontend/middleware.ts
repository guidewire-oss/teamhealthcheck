import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

function handleDocsRequest(request: NextRequest) {
  const { pathname } = request.nextUrl;

  
  if (/^\/docs\/INDEX\/?$/.test(pathname)) {
    return NextResponse.redirect(new URL('/docs/', request.url), 308);
  }

  // MkDocs pages use relative asset links (e.g. "assets/main.css"), so a
  // page must be served from its real trailing-slash directory URL or the
  // browser resolves assets against the wrong base and the page renders
  // unstyled. Static assets (css/js/xml/png/...) always have an extension
  // and are left alone; clean directory URLs get a trailing slash and are
  // then rewritten to the directory's index.html.
  const hasExtension = /\.[^/]+$/.test(pathname);
  if (hasExtension) {
    return NextResponse.next();
  }

  if (!pathname.endsWith('/')) {
    return NextResponse.redirect(new URL(pathname + '/', request.url), 308);
  }

  return NextResponse.rewrite(new URL(pathname + 'index.html', request.url));
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;
  if (pathname === '/docs' || pathname.startsWith('/docs/')) {
    return handleDocsRequest(request);
  }

  const userCookie = request.cookies.get('user');
  const isLoginPage = request.nextUrl.pathname === '/login';
  const isPublicPath = request.nextUrl.pathname === '/' || isLoginPage;
  const isSurveyPage = request.nextUrl.pathname === '/survey';

  if (!userCookie && !isPublicPath) {
    return NextResponse.redirect(new URL('/login', request.url));
  }

  if (userCookie) {
    const user = JSON.parse(userCookie.value);
    // Map API response format: hierarchyLevel or hierarchyLevelId
    const hierarchyLevelId = user.hierarchyLevel || user.hierarchyLevelId;

    // Redirect from login page based on hierarchy level
    if (isLoginPage) {
      if (hierarchyLevelId === 'admin' || hierarchyLevelId === 'level-admin') {
        return NextResponse.redirect(new URL('/admin', request.url));
      } else if (hierarchyLevelId === 'level-1' || hierarchyLevelId === 'level-2' || hierarchyLevelId === 'level-3') {
        // VP, Director, Manager → /manager
        return NextResponse.redirect(new URL('/manager', request.url));
      } else if (hierarchyLevelId === 'level-4') {
        // Team Lead → /dashboard (their team view)
        return NextResponse.redirect(new URL('/dashboard', request.url));
      } else if (hierarchyLevelId === 'level-5') {
        // Team Member → /home (member home page with history and survey button)
        return NextResponse.redirect(new URL('/home', request.url));
      }
    }

    // Enforce survey page access control
    if (isSurveyPage) {
      const canTakeSurvey = user.canTakeSurvey === true;
      if (!canTakeSurvey) {
        // Redirect based on role
        if (hierarchyLevelId === 'admin' || hierarchyLevelId === 'level-admin') {
          return NextResponse.redirect(new URL('/admin', request.url));
        } else if (hierarchyLevelId === 'level-4') {
          return NextResponse.redirect(new URL('/dashboard', request.url));
        } else if (hierarchyLevelId === 'level-5') {
          return NextResponse.redirect(new URL('/home', request.url));
        } else {
          // Manager, Director, VP → /manager
          return NextResponse.redirect(new URL('/manager', request.url));
        }
      }
    }
  }

  return NextResponse.next();
}

export const config = {
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'],
};