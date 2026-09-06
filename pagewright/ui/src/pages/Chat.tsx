import { getErrorMessage } from '../utils/errors';
import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { VersionsList } from '../components/VersionsList';
import { ChatMessage } from '../components/ChatMessage';
import { BuildHistory } from '../components/BuildHistory';
import { FileAttachment } from '../components/FileAttachment';
import { apiClient } from '../api/client';
import { createSubmissionIdentity, isRejectedSubmission } from '../api/submission';
import './Chat.css';

interface Message {
  id: string;
  text: string;
  sender: 'user' | 'agent';
  timestamp: Date;
}

export const Chat: React.FC = () => {
  const { fqdn } = useParams<{ fqdn: string }>();
  return <ChatSession key={fqdn} fqdn={fqdn!} />;
};

const ChatSession: React.FC<{ fqdn: string }> = ({ fqdn }) => {
  const [messages, setMessages] = useState<Message[]>([]);
  const [inputText, setInputText] = useState('');
  const [files, setFiles] = useState<File[]>([]);
  const [conversationId, setConversationId] = useState<string | undefined>();
  const [isLoading, setIsLoading] = useState(false);
  const [versionRefresh, setVersionRefresh] = useState(0);
  const [historyRefresh, setHistoryRefresh] = useState(0);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const submission = useRef(createSubmissionIdentity());

  const refreshCompletedVersions = useCallback(() => setVersionRefresh(n => n + 1), []);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const handleSend = async () => {
    if (!inputText.trim() && files.length === 0) return;
    const requestKey = submission.current.begin({ fqdn: fqdn!, message: inputText, conversation_id: conversationId, files });
    if (!requestKey) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      text: inputText,
      sender: 'user',
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMessage]);
    setIsLoading(true);

    try {
      const response = await apiClient.build(fqdn!, {
        message: inputText,
        conversation_id: conversationId,
        files,
        requestKey,
      });
      submission.current.finish('success');

      if ('question' in response) {
        // Agent needs clarification
        setConversationId(response.conversation_id);
        setMessages((prev) => [
          ...prev,
          {
            id: Date.now().toString() + '-q',
            text: response.question || '',
            sender: 'agent',
            timestamp: new Date(),
          },
        ]);
      } else {
        // An idempotent replay may already contain a terminal job status.
        setConversationId(undefined);
        if (response.status === 'completed') setVersionRefresh(prev => prev + 1);
        setMessages((prev) => [
          ...prev,
          {
            id: Date.now().toString() + '-j',
            text: response.status === 'failed'
              ? `✗ Build failed: ${response.error_message}. Based on ${response.source_version}.`
              : response.status === 'completed'
                ? `✓ Build completed! Version ${response.target_version} is ready. Based on ${response.source_version}.`
                : `Build submitted from version ${response.source_version}. Follow Build history for its current status.`,
            sender: 'agent',
            timestamp: new Date(),
          },
        ]);
      }

      setInputText('');
      setFiles([]);
    } catch (err: unknown) {
      submission.current.finish(isRejectedSubmission(err) ? 'rejected' : 'uncertain');
      setMessages((prev) => [
        ...prev,
        {
          id: Date.now().toString() + '-e',
          text: `Error: ${getErrorMessage(err, 'Failed to send message')}`,
          sender: 'agent',
          timestamp: new Date(),
        },
      ]);
    } finally {
      setIsLoading(false);
      setVersionRefresh(prev => prev + 1);
      setHistoryRefresh(prev => prev + 1);
    }
  };

  return (
    <Layout sidebar={<VersionsList fqdn={fqdn!} refresh={versionRefresh} />}>
      <div className="chat-container">
        <div className="chat-header">
          <h2>{fqdn}</h2>
          <div>
            <a href={`https://${fqdn}`} target="_blank" rel="noopener noreferrer" className="pure-button">
              View Live
            </a>
            <a href={`https://${fqdn}/preview`} target="_blank" rel="noopener noreferrer" className="pure-button">
              View Preview
            </a>
          </div>
        </div>

        <div className="chat-messages">
          <BuildHistory key={historyRefresh} fqdn={fqdn} onCompleted={refreshCompletedVersions} />
          {messages.length === 0 && (
            <div className="empty-chat">
              <p>Start building your site! Describe what you'd like to change.</p>
            </div>
          )}
          {messages.map((msg) => (
            <ChatMessage key={msg.id} message={msg} />
          ))}
          <div ref={messagesEndRef} />
        </div>

        <div className="chat-input">
          <FileAttachment files={files} onFilesChange={setFiles} disabled={isLoading} />
          <textarea
            value={inputText}
            onChange={(e) => setInputText(e.target.value)}
            onKeyPress={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Describe what you'd like to change..."
            disabled={isLoading}
            rows={3}
          />
          <button
            onClick={handleSend}
            disabled={isLoading || (!inputText.trim() && files.length === 0)}
            className="pure-button pure-button-primary"
          >
            {isLoading ? 'Sending...' : 'Send'}
          </button>
        </div>
      </div>
    </Layout>
  );
};
