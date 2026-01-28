import { createBrowserRouter, Navigate } from 'react-router-dom'
import { AppLayout } from './layout/AppLayout'
import { LoginPage } from '../pages/Login'
import { AnalyticsStatsPage } from '../pages/analytics/Stats'
import { AnalyticsSettlementsPage } from '../pages/analytics/Settlements'
import { AnalyticsExportsPage } from '../pages/analytics/Exports'
import { AlarmsPage } from '../pages/Alarms'

export const appRouter = createBrowserRouter([
  {
    path: '/',
    element: <Navigate to="/login" replace />
  },
  {
    path: '/login',
    element: <LoginPage />
  },
  {
    path: '/app',
    element: <AppLayout />,
    children: [
      {
        index: true,
        element: <Navigate to="analytics/stats" replace />
      },
      {
        path: 'analytics/stats',
        element: <AnalyticsStatsPage />
      },
      {
        path: 'analytics/settlements',
        element: <AnalyticsSettlementsPage />
      },
      {
        path: 'analytics/exports',
        element: <AnalyticsExportsPage />
      },
      {
        path: 'alarms',
        element: <AlarmsPage />
      }
    ]
  },
  {
    path: '/app/layout',
    element: <AppLayout />
  },
  {
    path: '*',
    element: <Navigate to="/login" replace />
  }
])
