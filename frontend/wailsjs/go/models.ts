export namespace main {
	
	export class ManifestRuntimeInfo {
	    xtts_site: string;
	    xtts_src: string;
	    extra_site_packages: string[];
	
	    static createFrom(source: any = {}) {
	        return new ManifestRuntimeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.xtts_site = source["xtts_site"];
	        this.xtts_src = source["xtts_src"];
	        this.extra_site_packages = source["extra_site_packages"];
	    }
	}
	export class SynthesisManifest {
	    input_transcript: string;
	    output_audio: string;
	    generated_with: string;
	    style: string;
	    device: string;
	    sample_rate: number;
	    pause_ms: number;
	    intra_turn_pause_ms: number;
	    preserve_timing: boolean;
	    add_conversation_markers: boolean;
	    speed: number;
	    speakers: Record<string, string>;
	    reference_audio: string;
	    speaker_reference_files: Record<string, Array<string>>;
	    speaker_reference_segments: Record<string, Array<ReferenceSegment>>;
	    available_speakers_sample: string[];
	    turns: number;
	    runtime_paths: ManifestRuntimeInfo;
	    manifest_path?: string;
	
	    static createFrom(source: any = {}) {
	        return new SynthesisManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.input_transcript = source["input_transcript"];
	        this.output_audio = source["output_audio"];
	        this.generated_with = source["generated_with"];
	        this.style = source["style"];
	        this.device = source["device"];
	        this.sample_rate = source["sample_rate"];
	        this.pause_ms = source["pause_ms"];
	        this.intra_turn_pause_ms = source["intra_turn_pause_ms"];
	        this.preserve_timing = source["preserve_timing"];
	        this.add_conversation_markers = source["add_conversation_markers"];
	        this.speed = source["speed"];
	        this.speakers = source["speakers"];
	        this.reference_audio = source["reference_audio"];
	        this.speaker_reference_files = source["speaker_reference_files"];
	        this.speaker_reference_segments = this.convertValues(source["speaker_reference_segments"], Array<ReferenceSegment>, true);
	        this.available_speakers_sample = source["available_speakers_sample"];
	        this.turns = source["turns"];
	        this.runtime_paths = this.convertValues(source["runtime_paths"], ManifestRuntimeInfo);
	        this.manifest_path = source["manifest_path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RuntimeInfo {
	    pythonExe: string;
	    pythonPath: string;
	    hfHome: string;
	    xttsSite: string;
	    xttsSrc: string;
	    extraSitePackages: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pythonExe = source["pythonExe"];
	        this.pythonPath = source["pythonPath"];
	        this.hfHome = source["hfHome"];
	        this.xttsSite = source["xttsSite"];
	        this.xttsSrc = source["xttsSrc"];
	        this.extraSitePackages = source["extraSitePackages"];
	    }
	}
	export class SynthesisOptions {
	    transcriptPath: string;
	    referenceAudioPath: string;
	    outputDir: string;
	    outputBaseName: string;
	    style: string;
	    addConversationMarkers: boolean;
	    preserveTiming: boolean;
	    coquiTOSAgreed: boolean;
	    language: string;
	    pauseMs: number;
	    intraTurnPauseMs: number;
	    speed: number;
	    maxCharsPerUtterance: number;
	    maxSentencesPerUtterance: number;
	    femaleSpeaker: string;
	    maleSpeaker: string;
	
	    static createFrom(source: any = {}) {
	        return new SynthesisOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.transcriptPath = source["transcriptPath"];
	        this.referenceAudioPath = source["referenceAudioPath"];
	        this.outputDir = source["outputDir"];
	        this.outputBaseName = source["outputBaseName"];
	        this.style = source["style"];
	        this.addConversationMarkers = source["addConversationMarkers"];
	        this.preserveTiming = source["preserveTiming"];
	        this.coquiTOSAgreed = source["coquiTOSAgreed"];
	        this.language = source["language"];
	        this.pauseMs = source["pauseMs"];
	        this.intraTurnPauseMs = source["intraTurnPauseMs"];
	        this.speed = source["speed"];
	        this.maxCharsPerUtterance = source["maxCharsPerUtterance"];
	        this.maxSentencesPerUtterance = source["maxSentencesPerUtterance"];
	        this.femaleSpeaker = source["femaleSpeaker"];
	        this.maleSpeaker = source["maleSpeaker"];
	    }
	}
	export class TranslationManifest {
	    inputAudio: string;
	    outputDir: string;
	    generatedAt: string;
	    turns: number;
	    segments: number;
	    manifest: string;
	    files: OutputFiles;
	
	    static createFrom(source: any = {}) {
	        return new TranslationManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inputAudio = source["inputAudio"];
	        this.outputDir = source["outputDir"];
	        this.generatedAt = source["generatedAt"];
	        this.turns = source["turns"];
	        this.segments = source["segments"];
	        this.manifest = source["manifest"];
	        this.files = this.convertValues(source["files"], OutputFiles);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReviewManifest {
	    inputAudio: string;
	    outputDir: string;
	    generatedAt: string;
	    turns: number;
	    issues: number;
	    summary: string;
	    manifest: string;
	    files: OutputFiles;
	
	    static createFrom(source: any = {}) {
	        return new ReviewManifest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inputAudio = source["inputAudio"];
	        this.outputDir = source["outputDir"];
	        this.generatedAt = source["generatedAt"];
	        this.turns = source["turns"];
	        this.issues = source["issues"];
	        this.summary = source["summary"];
	        this.manifest = source["manifest"];
	        this.files = this.convertValues(source["files"], OutputFiles);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReviewIssue {
	    category: string;
	    severity: string;
	    sourceText: string;
	    suggestion: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewIssue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.severity = source["severity"];
	        this.sourceText = source["sourceText"];
	        this.suggestion = source["suggestion"];
	        this.reason = source["reason"];
	    }
	}
	export class ReviewTurn {
	    turnIndex: number;
	    speaker: string;
	    start: number;
	    end: number;
	    startTs: string;
	    endTs: string;
	    originalText: string;
	    reviewedText: string;
	    issues: ReviewIssue[];
	
	    static createFrom(source: any = {}) {
	        return new ReviewTurn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.turnIndex = source["turnIndex"];
	        this.speaker = source["speaker"];
	        this.start = source["start"];
	        this.end = source["end"];
	        this.startTs = source["startTs"];
	        this.endTs = source["endTs"];
	        this.originalText = source["originalText"];
	        this.reviewedText = source["reviewedText"];
	        this.issues = this.convertValues(source["issues"], ReviewIssue);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReviewDraft {
	    summary: string;
	    issueCount: number;
	    turns: ReviewTurn[];
	
	    static createFrom(source: any = {}) {
	        return new ReviewDraft(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.issueCount = source["issueCount"];
	        this.turns = this.convertValues(source["turns"], ReviewTurn);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OutputFiles {
	    englishJson: string;
	    englishTxt: string;
	    englishSrt: string;
	    chineseJson: string;
	    chineseTxt: string;
	    reviewJson: string;
	    reviewTxt: string;
	    reviewManifest: string;
	    resultManifest: string;
	    outputAudio: string;
	
	    static createFrom(source: any = {}) {
	        return new OutputFiles(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.englishJson = source["englishJson"];
	        this.englishTxt = source["englishTxt"];
	        this.englishSrt = source["englishSrt"];
	        this.chineseJson = source["chineseJson"];
	        this.chineseTxt = source["chineseTxt"];
	        this.reviewJson = source["reviewJson"];
	        this.reviewTxt = source["reviewTxt"];
	        this.reviewManifest = source["reviewManifest"];
	        this.resultManifest = source["resultManifest"];
	        this.outputAudio = source["outputAudio"];
	    }
	}
	export class JobState {
	    running: boolean;
	    stage: string;
	    status: string;
	    message: string;
	    progress: number;
	    error: string;
	    audioPath: string;
	    referenceAudioPath: string;
	    outputDir: string;
	    englishTranscriptPath: string;
	    outputAudioPath: string;
	    manifestPath: string;
	    logs: string[];
	    commandPreview: string;
	    files: OutputFiles;
	    review: ReviewDraft;
	    reviewManifest: ReviewManifest;
	    translation: TranslationManifest;
	    options: SynthesisOptions;
	    runtime: RuntimeInfo;
	    result: SynthesisManifest;
	
	    static createFrom(source: any = {}) {
	        return new JobState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.stage = source["stage"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.progress = source["progress"];
	        this.error = source["error"];
	        this.audioPath = source["audioPath"];
	        this.referenceAudioPath = source["referenceAudioPath"];
	        this.outputDir = source["outputDir"];
	        this.englishTranscriptPath = source["englishTranscriptPath"];
	        this.outputAudioPath = source["outputAudioPath"];
	        this.manifestPath = source["manifestPath"];
	        this.logs = source["logs"];
	        this.commandPreview = source["commandPreview"];
	        this.files = this.convertValues(source["files"], OutputFiles);
	        this.review = this.convertValues(source["review"], ReviewDraft);
	        this.reviewManifest = this.convertValues(source["reviewManifest"], ReviewManifest);
	        this.translation = this.convertValues(source["translation"], TranslationManifest);
	        this.options = this.convertValues(source["options"], SynthesisOptions);
	        this.runtime = this.convertValues(source["runtime"], RuntimeInfo);
	        this.result = this.convertValues(source["result"], SynthesisManifest);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class ReferenceSegment {
	    start: number;
	    end: number;
	    duration: number;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new ReferenceSegment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.duration = source["duration"];
	        this.text = source["text"];
	    }
	}
	
	
	
	
	
	
	

}

