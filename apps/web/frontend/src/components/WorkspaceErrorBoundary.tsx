import { Alert, Button, Space } from 'antd';
import { Component, type ErrorInfo, type ReactNode } from 'react';

interface WorkspaceErrorBoundaryProps {
  children: ReactNode;
  workspaceName: string;
  onReset?: () => void;
}

interface WorkspaceErrorBoundaryState {
  error?: Error;
}

export class WorkspaceErrorBoundary extends Component<WorkspaceErrorBoundaryProps, WorkspaceErrorBoundaryState> {
  state: WorkspaceErrorBoundaryState = {};

  static getDerivedStateFromError(error: Error): WorkspaceErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`${this.props.workspaceName}工作区渲染失败`, error, info);
  }

  private retry = () => {
    this.setState({ error: undefined });
  };

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <Alert
        type="error"
        showIcon
        message={`${this.props.workspaceName}页面加载失败`}
        description="任务数据异常不会影响其他工作区。你可以返回每日复盘后继续使用，或刷新页面重试。"
        action={(
          <Space>
            <Button onClick={this.props.onReset}>返回每日复盘</Button>
            <Button type="primary" onClick={this.retry}>重试</Button>
          </Space>
        )}
      />
    );
  }
}
