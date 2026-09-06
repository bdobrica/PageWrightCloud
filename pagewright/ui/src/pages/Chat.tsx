import { getErrorMessage } from '../utils/errors';
import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useParams } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { VersionsList } from '../components/VersionsList';
import { ChatMessage } from '../components/ChatMessage';
import { BuildHistory } from '../components/BuildHistory';
import { HostingLinks } from '../components/HostingLinks';
import type { Site } from '../types/api';
import { apiClient } from '../api/client';
import { createSubmissionIdentity, isRejectedSubmission } from '../api/submission';
import { useAuth } from '../contexts/auth';
import { readDraft, writeDraft, readOwner } from '../api/drafts';
import type { Draft } from '../api/drafts';
import './Chat.css';

interface Message {
  id: string;
  text: string;
  sender: 'user' | 'agent';
  timestamp: Date;
}

export const Chat: React.FC = () => {
  const { fqdn } = useParams<{ fqdn: string }>();
  const { user } = useAuth();
  return user ? <ChatSession key={JSON.stringify([user.id, fqdn])} owner={user.id} fqdn={fqdn!} /> : null;
};

const ChatSession: React.FC<{ fqdn: string; owner: string }> = ({ fqdn, owner }) => {
  const [restored] = useState(() => {
    try { return { draft: readDraft(sessionStorage, owner, fqdn), error: '' }; }
    catch { return { draft: { text: '' } as Draft, error: 'Draft storage is unavailable or invalid. Copy your text before leaving; sending is blocked until storage works or the saved draft is discarded.' }; }
  });
  const draft = useRef(restored.draft);
  const [draftError, setDraftError] = useState(restored.error);
  const [question, setQuestion] = useState(restored.draft.question);
  const [sendError, setSendError] = useState('');
  const [uncertain, setUncertain] = useState(!!restored.draft.identity);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const saveDraft = (value: Draft): boolean => {
    draft.current = value;
    try {
      writeDraft(sessionStorage, owner, fqdn, value);
      setDraftError('');
      return true;
    } catch {
      setDraftError('Draft could not be saved in this tab. Copy your text before leaving. Sending is blocked until storage works.');
      return false;
    }
  };
  const [messages, setMessages] = useState<Message[]>([]);
  const [inputText, setInputText] = useState(restored.draft.text);
  const [conversationId, setConversationId] = useState<string | undefined>(restored.draft.conversationId);
  const [isLoading, setIsLoading] = useState(false);
  const [versionRefresh, setVersionRefresh] = useState(0);
  const [historyRefresh, setHistoryRefresh] = useState(0);
  const [hostingRefresh, setHostingRefresh] = useState(0);
  const [hostingError, setHostingError] = useState('');
  const [site, setSite] = useState<Site | null>(null);
  const refreshHosting = useCallback(() => setHostingRefresh(n => n + 1), []);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setHostingError('');
    apiClient.getSite(fqdn, controller.signal).then(value => {
      if (active) setSite(value);
    }).catch(() => { if (active) setHostingError('Hosting state could not be refreshed; displayed links may be stale.'); });
    return () => { active = false; controller.abort(); };
  }, [fqdn, hostingRefresh]);
  const messagesRegion = useRef<HTMLDivElement>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const sendButton = useRef<HTMLButtonElement>(null);
  const wasSending = useRef(false);
  useEffect(() => {
    if (wasSending.current && !isLoading && document.activeElement === document.body) {
      if (composer.current && !composer.current.disabled) composer.current.focus();
      else sendButton.current?.focus();
    }
    wasSending.current = isLoading;
  }, [isLoading]);
  const submission = useRef(createSubmissionIdentity(undefined, restored.draft.identity));

  const refreshCompletedVersions = useCallback(() => setVersionRefresh(n => n + 1), []);

  useEffect(() => {
    const region = messagesRegion.current;
    if (region && region.scrollHeight - region.scrollTop - region.clientHeight < 160) {
      region.scrollTop = region.scrollHeight;
    }
  }, [messages]);

  const handleSend = async () => {
    if (!inputText.trim() || isLoading || (restored.error && draftError)) return;
    if (readOwner(localStorage) !== owner) {
      setSendError('The signed-in account changed. Reload before sending this draft.');
      return;
    }
    const requestKey = submission.current.begin({ fqdn: fqdn!, message: inputText, conversation_id: conversationId });
    if (!requestKey) return;
    if (!saveDraft({ ...draft.current, text: inputText, conversationId, identity: submission.current.snapshot() })) {
      submission.current.finish('rejected'); // No request was sent.
      draft.current.identity = undefined;
      return;
    }
    setSendError('');
    setUncertain(true);

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
        requestKey,
      }, owner);
      if (!mounted.current || readOwner(localStorage) !== owner) return;
      submission.current.finish('success');
      setUncertain(false);
      const nextDraft: Draft = 'question' in response
        ? { text: '', conversationId: response.conversation_id, question: response.question,
            original: draft.current.original ?? inputText }
        : { text: '' };
      saveDraft(nextDraft);
      setQuestion(nextDraft.question);

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
    } catch (err: unknown) {
      if (!mounted.current || readOwner(localStorage) !== owner) return;
      setSendError(getErrorMessage(err, 'Submission could not be confirmed. Check build history, then retry the same request.'));
      const rejected = isRejectedSubmission(err);
      setUncertain(!rejected);
      submission.current.finish(rejected ? 'rejected' : 'uncertain');
      saveDraft({ ...draft.current, identity: submission.current.snapshot() });
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
      if (mounted.current) {
        setIsLoading(false);
        setVersionRefresh(prev => prev + 1);
        setHistoryRefresh(prev => prev + 1);
      }
    }
  };

  const discardDraft = () => {
    if (isLoading || !window.confirm('Discard this saved draft and retry identity? An already submitted build may still run. Check build history before starting another request.')) return;
    if (!saveDraft({ text: '' })) return;
    submission.current.finish('success');
    setInputText(''); setConversationId(undefined); setQuestion(undefined);
    setUncertain(false); setSendError('');
  };
  const reloadDraft = () => {
    try {
      const value = readDraft(sessionStorage, owner, fqdn);
      draft.current = value;
      submission.current = createSubmissionIdentity(undefined, value.identity);
      setInputText(value.text); setConversationId(value.conversationId);
      setQuestion(value.question); setUncertain(!!value.identity); setDraftError('');
    } catch { setDraftError('Saved draft still cannot be read. No stored content was changed.'); }
  };
  const restartClarification = () => {
    if (!window.confirm('Start a new request with the saved original text and your answer? Check history first; this will use a new request identity.')) return;
    const text = [draft.current.original, inputText].filter(Boolean).join('\n\n');
    if (text.length > 50000) {
      setSendError('The combined request exceeds 50,000 characters. Copy the original and answer before discarding; shorten them for a new request.');
      return;
    }
    if (!saveDraft({ text })) return;
    submission.current.finish('success');
    setInputText(text); setConversationId(undefined); setQuestion(undefined); setUncertain(false); setSendError('');
  };

  return (
    <Layout sidebar={<VersionsList fqdn={fqdn!} refresh={versionRefresh} onDeployed={refreshHosting} site={site} />}>
      <div className="chat-container">
        <div className="chat-header">
          <h1>{fqdn}</h1>
          <div>
            <HostingLinks site={site} />
            <button onClick={refreshHosting}>Refresh hosting state</button>
            {hostingError && <p role="alert">{hostingError}</p>}
          </div>
        </div>

        <div className="chat-messages" ref={messagesRegion} tabIndex={0} role="region" aria-label="Conversation and build history">
          <BuildHistory key={historyRefresh} fqdn={fqdn} onCompleted={refreshCompletedVersions} />
          {messages.length === 0 && (
            <div className="empty-chat">
              <p>Start building your site! Describe what you'd like to change.</p>
            </div>
          )}
          {messages.map((msg) => (
            <ChatMessage key={msg.id} message={msg} />
          ))}
        </div>

        <div className="chat-input">
          <p>Text-only requests. Attachments are not supported in this MVP.</p>
          <p role="status">{isLoading ? 'Submitting request; keep this tab open…' : draftError ? 'Draft is not saved.' : uncertain ? 'Submission may already exist. Retry preserves its identity; nothing is sent automatically.' : 'Draft saved in this tab for this account and site. Signing out clears saved drafts.'}</p>
          {question && <div><p>Original request: {draft.current.original}</p><p>Clarification: {question}</p></div>}
          {draftError && <p role="alert">{draftError}</p>}
          {restored.error && draftError && <button onClick={reloadDraft}>Retry loading saved draft</button>}
          {sendError && <p role="alert">{sendError}</p>}
          <textarea
            ref={composer}
            value={inputText}
            onChange={(e) => {
              setInputText(e.target.value);
              if (!restored.error || !draftError) saveDraft({ ...draft.current, text: e.target.value });
            }}
            maxLength={50000}
            aria-label="Build request"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing && !e.repeat) {
                e.preventDefault();
                handleSend();
              }
            }}
            placeholder="Describe what you'd like to change..."
            disabled={isLoading || uncertain || !!(restored.error && draftError)}
            rows={3}
          />
          <button
            ref={sendButton}
            onClick={handleSend}
            disabled={isLoading || !inputText.trim()}
            className="pure-button pure-button-primary"
          >
            {isLoading ? 'Sending...' : uncertain ? 'Retry same request' : 'Send'}
          </button>
          <button onClick={discardDraft} disabled={isLoading}>Discard draft</button>
          {conversationId && sendError && <button onClick={restartClarification} disabled={isLoading}>Restart clarification as a new request</button>}
        </div>
      </div>
    </Layout>
  );
};
