import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { 
  LayoutDashboard, GitBranch, Settings, MessageSquare, Copy,
  ChevronLeft, ChevronRight, LogOut
} from 'lucide-react'
import { cn } from '../../lib/utils'
import { useUIStore } from '../../store/ui'
import { api } from '../../api/client'
import { AUTH_SESSION_QUERY_KEY, useAuthSession } from '../../lib/auth'
import type { AuthSession } from '../../types'

const navItems = [
  { icon: LayoutDashboard, label: 'Dashboard', path: '/' },
  { icon: GitBranch, label: 'Pipelines', path: '/pipelines' },
  { icon: Copy, label: 'Templates', path: '/templates' },
  { icon: MessageSquare, label: 'LLM Chat', path: '/chat' },
  { icon: Settings, label: 'Settings', path: '/settings' },
]

export default function Sidebar() {
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const sessionQuery = useAuthSession()
  const { sidebarCollapsed, toggleSidebar, addToast } = useUIStore()
  const username = sessionQuery.data?.username ?? ''
  const userInitial = username.trim().charAt(0).toUpperCase() || '?'

  async function handleLogout() {
    try {
      await api.auth.logout()
      queryClient.setQueryData<AuthSession | null>(AUTH_SESSION_QUERY_KEY, null)
      navigate('/login', { replace: true })
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to sign out',
        message: err instanceof Error ? err.message : 'Unknown error',
      })
    }
  }

  return (
    <div className={cn(
      'flex flex-col bg-bg-elevated border-r border-border transition-all duration-300',
      sidebarCollapsed ? 'w-16' : 'w-64'
    )}>
      <div className="flex items-center h-14 px-4 border-b border-border">
        {!sidebarCollapsed && (
          <span className="text-lg font-bold text-accent">Automator</span>
        )}
        {sidebarCollapsed && (
          <span className="text-lg font-bold text-accent mx-auto">A</span>
        )}
      </div>

      <nav className="flex-1 py-4 px-2 space-y-1">
        {navItems.map(({ icon: Icon, label, path }) => {
          const isActive = location.pathname === path || (path !== '/' && location.pathname.startsWith(path))
          return (
            <Link
              key={path}
              to={path}
              className={cn(
                'flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors',
                isActive
                  ? 'bg-accent/10 text-accent'
                  : 'text-text-muted hover:bg-bg-overlay hover:text-text'
              )}
            >
              <Icon className="w-5 h-5 flex-shrink-0" />
              {!sidebarCollapsed && <span>{label}</span>}
            </Link>
          )
        })}
      </nav>

      <div className="space-y-2 border-t border-border p-2">
        {sessionQuery.data && (
          <div className={cn(
            'rounded-lg border border-border bg-bg-input/70 px-3 py-2',
            sidebarCollapsed ? 'flex justify-center px-2 py-3' : '',
          )}>
            {sidebarCollapsed ? (
              <div
                className="flex h-9 w-9 items-center justify-center rounded-full border border-accent/30 bg-accent/12 text-sm font-semibold text-accent"
                title={username}
                aria-label={`Signed in as ${username}`}
              >
                {userInitial}
              </div>
            ) : (
              <>
                <div className="text-[11px] uppercase tracking-[0.18em] text-text-dimmed">User</div>
              <div className="mt-1 truncate text-sm font-medium text-text">
                {username}
              </div>
              </>
            )}
          </div>
        )}
        <button
          onClick={handleLogout}
          className="flex items-center gap-3 w-full px-3 py-2.5 rounded-lg text-sm text-text-muted hover:text-text hover:bg-bg-overlay transition-colors"
        >
          {sidebarCollapsed
            ? <LogOut className="w-5 h-5 mx-auto" />
            : <><LogOut className="w-5 h-5" /><span>Sign out</span></>
          }
        </button>
        <button
          onClick={toggleSidebar}
          className="flex items-center gap-3 w-full px-3 py-2.5 rounded-lg text-sm text-text-muted hover:text-text hover:bg-bg-overlay transition-colors"
        >
          {sidebarCollapsed 
            ? <ChevronRight className="w-5 h-5 mx-auto" />
            : <><ChevronLeft className="w-5 h-5" /><span>Collapse</span></>
          }
        </button>
      </div>
    </div>
  )
}
