CREATE TABLE public.organization (
    user_id character varying NOT NULL,
    id character varying NOT NULL,
    payload jsonb NOT NULL
);

ALTER TABLE ONLY public.organization
    ADD CONSTRAINT organization_pkey PRIMARY KEY (user_id, id);

CREATE TABLE public.user_profile (
    user_id character varying NOT NULL,
    ref_id_1 character varying, -- refer to organization
    id character varying NOT NULL,
    payload jsonb NOT NULL
);

ALTER TABLE ONLY public.user_profile
    ADD CONSTRAINT user_profile_pkey PRIMARY KEY (user_id, id, ref_id_1);

CREATE TABLE public.user_profile_thumbnail (
    user_id character varying NOT NULL,
    ref_id_1 character varying NOT NULL, -- refer to user_profile
    id character varying NOT NULL,
    payload jsonb NOT NULL
);

ALTER TABLE ONLY public.user_profile_thumbnail
    ADD CONSTRAINT user_profile_thumbnail_pkey PRIMARY KEY (user_id, id, ref_id_1);

CREATE TABLE public.user_page (
    user_id character varying NOT NULL,
    ref_id_1 character varying, -- refer to organization (allows to be queried by organization)
    ref_id_2 character varying, -- refer to user_profile
    id character varying NOT NULL,
    payload jsonb NOT NULL
);

ALTER TABLE ONLY public.user_page
    ADD CONSTRAINT user_page_pkey PRIMARY KEY (user_id, id, ref_id_1, ref_id_2);
