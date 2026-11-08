import type { TFunction } from 'i18next'
import { z } from 'zod'

import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'

export function createRoutingPolicySchema(t: TFunction) {
  return z.object({
    RetryTimes: z.number().int().min(0).max(99),
    chat_early_chunk_setting: z
      .object({
        mode: z.enum(['off', 'observe', 'enabled']),
        window_start_ms: z.number().int().min(0).max(29999),
        window_end_ms: z.number().int().min(1).max(30000),
        target_gap_ms: z.number().int().min(1).max(10000),
        min_samples: z.number().int().min(20).max(256),
        min_probability: z.number().min(0.5).max(1),
        max_age_seconds: z.number().int().min(60).max(86400),
      })
      .refine((value) => value.window_end_ms > value.window_start_ms, {
        message: t('The early chunk window must end after it starts.'),
        path: ['window_end_ms'],
      }),
    AutomaticRetryStatusCodes: z
      .string()
      .refine(
        (value) => parseHttpStatusCodeRules(value).ok,
        t('Invalid status code rules')
      ),
    chat_prefix_routing_setting: z.object({
      enabled: z.boolean(),
      max_entries: z.number().int().min(0).max(1000000),
      ttl_seconds: z.number().int().min(1).max(86400),
    }),
    channel_affinity_setting: z.object({
      enabled: z.boolean(),
      session_mode: z.enum(['', 'off', 'prefer', 'strict']),
      switch_on_success: z.boolean(),
      keep_on_channel_disabled: z.boolean(),
      max_entries: z.number().int().min(0),
      default_ttl_seconds: z.number().int().min(0),
      rules: z.string().superRefine((value, context) => {
        try {
          const rules: unknown = JSON.parse(value)
          if (!Array.isArray(rules)) {
            context.addIssue({
              code: 'custom',
              message: t('Rules JSON must be an array'),
            })
          }
        } catch {
          context.addIssue({
            code: 'custom',
            message: t('Invalid rules JSON format'),
          })
        }
      }),
    }),
  })
}

export type RoutingPolicyFormValues = z.infer<
  ReturnType<typeof createRoutingPolicySchema>
>

export function routingPolicyFormValues(
  options: Record<string, string>
): RoutingPolicyFormValues {
  return {
    RetryTimes: Number(options.RetryTimes),
    chat_early_chunk_setting: {
      mode: (options['chat_early_chunk_setting.mode'] ?? 'off') as
        | 'off'
        | 'observe'
        | 'enabled',
      window_start_ms: Number(
        options['chat_early_chunk_setting.window_start_ms'] ?? 4500
      ),
      window_end_ms: Number(
        options['chat_early_chunk_setting.window_end_ms'] ?? 5500
      ),
      target_gap_ms: Number(
        options['chat_early_chunk_setting.target_gap_ms'] ?? 1000
      ),
      min_samples: Number(
        options['chat_early_chunk_setting.min_samples'] ?? 100
      ),
      min_probability: Number(
        options['chat_early_chunk_setting.min_probability'] ?? 0.95
      ),
      max_age_seconds: Number(
        options['chat_early_chunk_setting.max_age_seconds'] ?? 900
      ),
    },
    AutomaticRetryStatusCodes: options.AutomaticRetryStatusCodes,
    chat_prefix_routing_setting: {
      enabled: options['chat_prefix_routing_setting.enabled'] !== 'false',
      max_entries: Number(
        options['chat_prefix_routing_setting.max_entries'] ?? 40960
      ),
      ttl_seconds: Number(
        options['chat_prefix_routing_setting.ttl_seconds'] ?? 300
      ),
    },
    channel_affinity_setting: {
      enabled: options['channel_affinity_setting.enabled'] === 'true',
      session_mode: (options['channel_affinity_setting.session_mode'] ||
        '') as RoutingPolicyFormValues['channel_affinity_setting']['session_mode'],
      switch_on_success:
        options['channel_affinity_setting.switch_on_success'] === 'true',
      keep_on_channel_disabled:
        options['channel_affinity_setting.keep_on_channel_disabled'] === 'true',
      max_entries: Number(options['channel_affinity_setting.max_entries']),
      default_ttl_seconds: Number(
        options['channel_affinity_setting.default_ttl_seconds']
      ),
      rules: options['channel_affinity_setting.rules'] || '[]',
    },
  }
}

export function routingPolicyOptions(
  values: RoutingPolicyFormValues
): Record<string, string> {
  return {
    RetryTimes: String(values.RetryTimes),
    ...Object.fromEntries(
      Object.entries(values.chat_early_chunk_setting).map(([key, value]) => [
        `chat_early_chunk_setting.${key}`,
        String(value),
      ])
    ),
    AutomaticRetryStatusCodes: values.AutomaticRetryStatusCodes,
    'chat_prefix_routing_setting.enabled': String(
      values.chat_prefix_routing_setting.enabled
    ),
    'chat_prefix_routing_setting.max_entries': String(
      values.chat_prefix_routing_setting.max_entries
    ),
    'chat_prefix_routing_setting.ttl_seconds': String(
      values.chat_prefix_routing_setting.ttl_seconds
    ),
    ...Object.fromEntries(
      Object.entries(values.channel_affinity_setting).map(([key, value]) => [
        `channel_affinity_setting.${key}`,
        String(value),
      ])
    ),
  }
}
